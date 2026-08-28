use super::{FRAME_LIMIT, encode_path};
use anyhow::{Context, Result, bail};
use async_trait::async_trait;
use bytes::Bytes;
use rc_api_client::{ApiClient, WebRtcAnswer};
use rc_mesh::{EncryptedFrameTransport, FrameTransportError};
use rc_protocol::{ControlTransportMessage, IceServer};
use serde::Serialize;
use std::sync::Arc;
use tokio::sync::{Mutex, mpsc};
use webrtc::{
    api::APIBuilder,
    data_channel::{RTCDataChannel, data_channel_message::DataChannelMessage},
    ice_transport::ice_server::RTCIceServer,
    peer_connection::{
        RTCPeerConnection, configuration::RTCConfiguration,
        sdp::session_description::RTCSessionDescription,
    },
};

pub(super) async fn open_webrtc(
    api: &ApiClient,
    device_id: &str,
    session_id: &str,
    servers: &[IceServer],
) -> Result<(
    Arc<RTCPeerConnection>,
    Arc<dyn EncryptedFrameTransport>,
    mpsc::Receiver<ControlTransportMessage>,
)> {
    let config = RTCConfiguration {
        ice_servers: servers
            .iter()
            .map(|server| RTCIceServer {
                urls: server.urls.clone(),
                username: server.username.clone(),
                credential: server.credential.clone(),
            })
            .collect(),
        ..Default::default()
    };
    let peer = Arc::new(
        APIBuilder::new()
            .build()
            .new_peer_connection(config)
            .await?,
    );
    let channel = peer.create_data_channel("rc-control", None).await?;
    let (tx, rx) = mpsc::channel(128);
    let expected = session_id.to_owned();
    let message_channel = channel.clone();
    channel.on_message(Box::new(move |message: DataChannelMessage| {
        let tx = tx.clone();
        let expected = expected.clone();
        let channel = message_channel.clone();
        Box::pin(async move {
            if !message.is_string || message.data.len() > FRAME_LIMIT + 512 {
                let _ = channel.close().await;
                return;
            }
            let Ok(frame) = serde_json::from_slice::<ControlTransportMessage>(&message.data) else {
                let _ = channel.close().await;
                return;
            };
            let valid = matches!(&frame, ControlTransportMessage::Frame { session_id, .. } if session_id == &expected);
            if !valid || tx.try_send(frame).is_err() { let _ = channel.close().await; }
        })
    }));
    let (open_tx, open_rx) = tokio::sync::oneshot::channel();
    let open_tx = Arc::new(Mutex::new(Some(open_tx)));
    channel.on_open(Box::new(move || {
        let open_tx = open_tx.clone();
        Box::pin(async move {
            if let Some(tx) = open_tx.lock().await.take() {
                let _ = tx.send(());
            }
        })
    }));

    let offer = peer.create_offer(None).await?;
    let mut gather = peer.gathering_complete_promise().await;
    peer.set_local_description(offer).await?;
    let _ = tokio::time::timeout(std::time::Duration::from_secs(15), gather.recv()).await;
    let sdp = peer
        .local_description()
        .await
        .ok_or_else(|| anyhow::anyhow!("WebRTC offer unavailable"))?
        .sdp;
    if !sdp.contains("a=candidate:") {
        bail!("WebRTC ICE gathering produced no candidates");
    }
    #[derive(Serialize)]
    #[serde(rename_all = "camelCase")]
    struct Offer<'a> {
        device_id: &'a str,
        sdp: &'a str,
    }
    let answer: WebRtcAnswer = api
        .post(
            &format!("/api/v1/control/{}/webrtc", encode_path(session_id)),
            &Offer {
                device_id,
                sdp: &sdp,
            },
        )
        .await?;
    if answer.sdp.is_empty() {
        bail!("WebRTC answer rejected");
    }
    peer.set_remote_description(RTCSessionDescription::answer(answer.sdp)?)
        .await?;
    tokio::time::timeout(std::time::Duration::from_secs(8), open_rx)
        .await
        .context("WebRTC connection timed out")??;
    Ok((peer, Arc::new(WebRtcFrameTransport { channel }), rx))
}

struct WebRtcFrameTransport {
    channel: Arc<RTCDataChannel>,
}

#[async_trait]
impl EncryptedFrameTransport for WebRtcFrameTransport {
    async fn send(&self, frame: Bytes) -> Result<(), FrameTransportError> {
        let text = String::from_utf8(frame.to_vec()).map_err(|_| FrameTransportError::Rejected)?;
        self.channel
            .send_text(text)
            .await
            .map(|_| ())
            .map_err(|_| FrameTransportError::Closed)
    }
}
