use super::fixture::recv_hosted;
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_crypto::encrypt_frame;
use rc_node::ControlManager;
use rc_protocol::{ControlMessage, ControlTransportMessage, NodeToServer, ServerToNode};
use std::{sync::Arc, time::Duration};
use tokio::sync::{mpsc, oneshot};
use webrtc::{
    api::APIBuilder,
    data_channel::{RTCDataChannel, data_channel_message::DataChannelMessage},
    peer_connection::{
        RTCPeerConnection, configuration::RTCConfiguration,
        sdp::session_description::RTCSessionDescription,
    },
};

pub(super) struct PeerHarness {
    pub(super) peer: Arc<RTCPeerConnection>,
    pub(super) channel: Arc<RTCDataChannel>,
    pub(super) frames: mpsc::UnboundedReceiver<ControlTransportMessage>,
}

pub(super) async fn connect_peer(
    control: &ControlManager,
    hosted: &mut mpsc::UnboundedReceiver<NodeToServer>,
    session_id: &str,
) -> anyhow::Result<PeerHarness> {
    let peer = Arc::new(
        APIBuilder::new()
            .build()
            .new_peer_connection(RTCConfiguration::default())
            .await?,
    );
    let channel = peer.create_data_channel("rc-control", None).await?;
    let (opened_tx, opened_rx) = oneshot::channel();
    let opened_tx = Arc::new(tokio::sync::Mutex::new(Some(opened_tx)));
    channel.on_open(Box::new(move || {
        let opened = opened_tx.clone();
        Box::pin(async move {
            if let Some(tx) = opened.lock().await.take() {
                let _ = tx.send(());
            }
        })
    }));
    let (frames_tx, frames) = mpsc::unbounded_channel();
    channel.on_message(Box::new(move |message: DataChannelMessage| {
        let frames = frames_tx.clone();
        Box::pin(async move {
            if let Ok(frame) = serde_json::from_slice::<ControlTransportMessage>(&message.data) {
                let _ = frames.send(frame);
            }
        })
    }));

    let offer = peer.create_offer(None).await?;
    let mut gathered = peer.gathering_complete_promise().await;
    peer.set_local_description(offer).await?;
    tokio::time::timeout(Duration::from_secs(5), gathered.recv()).await?;
    let offer_sdp = peer
        .local_description()
        .await
        .ok_or_else(|| anyhow::anyhow!("missing client offer"))?
        .sdp;
    control
        .handle(
            "https://rc.example.test",
            ServerToNode::ControlWebrtcOffer {
                request_id: "webrtc".into(),
                session_id: session_id.to_owned(),
                sdp: offer_sdp,
                ice_servers: Vec::new(),
            },
        )
        .await;
    let answer = match recv_hosted(hosted).await? {
        NodeToServer::ControlWebrtcAnswer {
            request_id,
            session_id: answer_session,
            sdp,
        } => {
            assert_eq!(request_id, "webrtc");
            assert_eq!(answer_session, session_id);
            sdp
        }
        other => anyhow::bail!("unexpected WebRTC response: {other:?}"),
    };
    peer.set_remote_description(RTCSessionDescription::answer(answer)?)
        .await?;
    tokio::time::timeout(Duration::from_secs(5), opened_rx).await??;
    Ok(PeerHarness {
        peer,
        channel,
        frames,
    })
}

pub(super) async fn send_encrypted(
    channel: &RTCDataChannel,
    key: &[u8; 32],
    session_id: &str,
    sequence: u64,
    message: &ControlMessage,
) -> anyhow::Result<()> {
    let ciphertext = encrypt_frame(
        key,
        1,
        sequence,
        session_id,
        "c2n",
        &serde_json::to_vec(message)?,
    )?;
    channel
        .send_text(serde_json::to_string(&ControlTransportMessage::Frame {
            session_id: session_id.into(),
            sequence,
            ciphertext,
        })?)
        .await?;
    Ok(())
}

pub(super) async fn assert_encrypted_process_output(
    frames: &mut mpsc::UnboundedReceiver<ControlTransportMessage>,
    key: &[u8; 32],
    session_id: &str,
    process_id: &str,
    expected: &[u8],
) -> anyhow::Result<()> {
    let mut receive_sequence = 0_u64;
    let mut saw_expected = false;
    let mut saw_exit = false;
    while !saw_exit {
        let frame = tokio::time::timeout(Duration::from_secs(5), frames.recv())
            .await?
            .ok_or_else(|| anyhow::anyhow!("control channel closed early"))?;
        let ControlTransportMessage::Frame {
            session_id: frame_session,
            sequence,
            ciphertext,
        } = frame;
        assert_eq!(frame_session, session_id);
        assert_eq!(sequence, receive_sequence + 1);
        receive_sequence = sequence;
        let plaintext = rc_crypto::decrypt_frame(key, 2, sequence, session_id, "n2c", &ciphertext)?;
        match serde_json::from_slice::<ControlMessage>(&plaintext)? {
            ControlMessage::ProcessStdout { id, data } if id == process_id => {
                if URL_SAFE_NO_PAD.decode(data)? == expected {
                    saw_expected = true;
                }
            }
            ControlMessage::ProcessExit { id, .. } if id == process_id => saw_exit = true,
            _ => {}
        }
    }
    anyhow::ensure!(saw_expected, "encrypted process output was not delivered");
    Ok(())
}

pub(super) async fn assert_hosted_exit_without_plaintext(
    hosted: &mut mpsc::UnboundedReceiver<NodeToServer>,
    process_id: &str,
    forbidden: &str,
) -> anyhow::Result<()> {
    loop {
        let message = recv_hosted(hosted).await?;
        anyhow::ensure!(
            !serde_json::to_string(&message)?.contains(forbidden),
            "hosted channel exposed process plaintext"
        );
        if matches!(
            message,
            NodeToServer::ProcessExit {
                ref id,
                exit_code: 0,
                ..
            } if id == process_id
        ) {
            return Ok(());
        }
    }
}

pub(super) async fn wait_control_closed(
    hosted: &mut mpsc::UnboundedReceiver<NodeToServer>,
    session_id: &str,
) -> anyhow::Result<()> {
    loop {
        if matches!(
            recv_hosted(hosted).await?,
            NodeToServer::ControlClosed { session_id: ref id } if id == session_id
        ) {
            return Ok(());
        }
    }
}
