use super::{FRAME_LIMIT, encode_path};
use anyhow::{Context, Result, bail};
use async_trait::async_trait;
use bytes::Bytes;
use rc_api_client::{ApiClient, WebRtcAnswer};
use rc_mesh::{EncryptedFrameTransport, FrameTransportError};
use rc_protocol::{ControlIceAttempt, ControlIceMode, ControlTransportMessage, IceServer};
use serde::Serialize;
use std::sync::Arc;
use tokio::sync::{Mutex, mpsc};
use webrtc::{
    api::APIBuilder,
    data_channel::{RTCDataChannel, data_channel_message::DataChannelMessage},
    ice_transport::ice_server::RTCIceServer,
    peer_connection::{
        RTCPeerConnection, configuration::RTCConfiguration,
        policy::ice_transport_policy::RTCIceTransportPolicy,
        sdp::session_description::RTCSessionDescription,
    },
};

pub(super) async fn open_webrtc(
    api: &ApiClient,
    device_id: &str,
    session_id: &str,
    servers: &[IceServer],
    attempts: &[ControlIceAttempt],
) -> Result<(
    Arc<RTCPeerConnection>,
    Arc<dyn EncryptedFrameTransport>,
    mpsc::Receiver<ControlTransportMessage>,
)> {
    if attempts.first().map(|attempt| attempt.mode) != Some(ControlIceMode::Host) {
        bail!("invalid WebRTC attempt plan");
    }
    let mut last = None;
    for attempt in attempts {
        match open_attempt(api, device_id, session_id, servers, attempt).await {
            Ok(value) => return Ok(value),
            Err(error) => {
                last = Some(error);
                tokio::time::sleep(std::time::Duration::from_millis(u64::from(
                    attempt.retry_delay_ms,
                )))
                .await;
            }
        }
    }
    Err(last.unwrap_or_else(|| anyhow::anyhow!("WebRTC attempt plan is empty")))
}

async fn open_attempt(
    api: &ApiClient,
    device_id: &str,
    session_id: &str,
    servers: &[IceServer],
    attempt: &ControlIceAttempt,
) -> Result<(
    Arc<RTCPeerConnection>,
    Arc<dyn EncryptedFrameTransport>,
    mpsc::Receiver<ControlTransportMessage>,
)> {
    let config = RTCConfiguration {
        ice_servers: filtered_servers(servers, attempt.mode)
            .iter()
            .map(|server| RTCIceServer {
                urls: server.urls.clone(),
                username: server.username.clone(),
                credential: server.credential.clone(),
            })
            .collect(),
        ice_transport_policy: if attempt.mode == ControlIceMode::Relay {
            RTCIceTransportPolicy::Relay
        } else {
            RTCIceTransportPolicy::All
        },
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
    let _ = tokio::time::timeout(
        std::time::Duration::from_millis(u64::from(attempt.gather_timeout_ms)),
        gather.recv(),
    )
    .await;
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
        mode: ControlIceMode,
    }
    let answer: WebRtcAnswer = api
        .post(
            &format!("/api/v1/control/{}/webrtc", encode_path(session_id)),
            &Offer {
                device_id,
                sdp: &sdp,
                mode: attempt.mode,
            },
        )
        .await?;
    if answer.sdp.is_empty() {
        bail!("WebRTC answer rejected");
    }
    peer.set_remote_description(RTCSessionDescription::answer(answer.sdp)?)
        .await?;
    tokio::time::timeout(
        std::time::Duration::from_millis(u64::from(attempt.connect_timeout_ms)),
        open_rx,
    )
    .await
    .context("WebRTC connection timed out")??;
    Ok((peer, Arc::new(WebRtcFrameTransport { channel }), rx))
}

fn filtered_servers(servers: &[IceServer], mode: ControlIceMode) -> Vec<IceServer> {
    servers
        .iter()
        .filter_map(|server| {
            let urls = server
                .urls
                .iter()
                .filter(|url| match mode {
                    ControlIceMode::Host => false,
                    ControlIceMode::Stun => url.to_ascii_lowercase().starts_with("stun"),
                    ControlIceMode::Relay => url.to_ascii_lowercase().starts_with("turn"),
                })
                .cloned()
                .collect::<Vec<_>>();
            (!urls.is_empty()).then(|| IceServer {
                urls,
                username: server.username.clone(),
                credential: server.credential.clone(),
            })
        })
        .collect()
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
