use crate::{
    NodeState, sign_node_request,
    transport::webrtc::{complete_local_description, peer_connection},
};
use rc_protocol::{IceServer, NODE_CONTROL_MESSAGE_LIMIT, NodeToServer, ServerToNode};
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use tokio::sync::{mpsc, oneshot, watch};
use webrtc::{
    data_channel::{RTCDataChannel, data_channel_message::DataChannelMessage},
    peer_connection::{
        RTCPeerConnection, peer_connection_state::RTCPeerConnectionState,
        sdp::session_description::RTCSessionDescription,
    },
};

pub struct ServerTransport {
    peer: Arc<RTCPeerConnection>,
    channel: Arc<RTCDataChannel>,
    incoming: mpsc::Receiver<ServerToNode>,
    closed: watch::Receiver<bool>,
}

impl ServerTransport {
    pub async fn connect(server: &str, state: &NodeState) -> anyhow::Result<Self> {
        let servers = fetch_ice(server, state).await?;
        let peer = peer_connection(&servers, false).await?;
        let channel = peer.create_data_channel("rc-node", None).await?;
        let (incoming_tx, incoming) = mpsc::channel(256);
        channel.on_message(Box::new(move |message: DataChannelMessage| {
            let tx = incoming_tx.clone();
            Box::pin(async move {
                if !message.is_string || message.data.len() > NODE_CONTROL_MESSAGE_LIMIT {
                    return;
                }
                if let Ok(value) = serde_json::from_slice::<ServerToNode>(&message.data) {
                    let _ = tx.send(value).await;
                }
            })
        }));
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
        let (closed_tx, closed) = watch::channel(false);
        let channel_closed_tx = closed_tx.clone();
        channel.on_close(Box::new(move || {
            let tx = channel_closed_tx.clone();
            Box::pin(async move {
                let _ = tx.send(true);
            })
        }));
        peer.on_peer_connection_state_change(Box::new(move |state| {
            let tx = closed_tx.clone();
            Box::pin(async move {
                if matches!(
                    state,
                    RTCPeerConnectionState::Failed
                        | RTCPeerConnectionState::Closed
                        | RTCPeerConnectionState::Disconnected
                ) {
                    let _ = tx.send(true);
                }
            })
        }));

        let offer = peer.create_offer(None).await?;
        let sdp = complete_local_description(&peer, offer).await?;
        let answer = post_offer(server, state, &sdp).await?;
        peer.set_remote_description(RTCSessionDescription::answer(answer)?)
            .await?;
        tokio::time::timeout(std::time::Duration::from_secs(15), opened_rx)
            .await
            .map_err(|_| anyhow::anyhow!("Node WebRTC DataChannel timed out"))??;
        Ok(Self {
            peer,
            channel,
            incoming,
            closed,
        })
    }

    pub async fn send(&self, message: &NodeToServer) -> anyhow::Result<()> {
        let encoded = serde_json::to_string(message)?;
        if encoded.len() > NODE_CONTROL_MESSAGE_LIMIT {
            anyhow::bail!(
                "Node control message exceeds the {NODE_CONTROL_MESSAGE_LIMIT}-byte transport frame"
            );
        }
        self.channel.send_text(encoded).await?;
        Ok(())
    }

    pub async fn recv(&mut self) -> Option<ServerToNode> {
        self.incoming.recv().await
    }
    pub fn closed(&self) -> watch::Receiver<bool> {
        self.closed.clone()
    }
    pub async fn close(&self) {
        let _ = self.peer.close().await;
    }
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct IceResponse {
    ice_servers: Vec<IceServer>,
}

async fn fetch_ice(server: &str, state: &NodeState) -> anyhow::Result<Vec<IceServer>> {
    let path = "/api/v1/node/ice";
    let auth = sign_node_request(state, "GET", path, &[])?;
    let mut request =
        reqwest::Client::new().get(format!("{}{}", server.trim_end_matches('/'), path));
    for (name, value) in auth.headers() {
        request = request.header(name, value);
    }
    let response = request.send().await?;
    if !response.status().is_success() {
        anyhow::bail!("Node ICE request failed: {}", response.status());
    }
    Ok(response.json::<IceResponse>().await?.ice_servers)
}

async fn post_offer(server: &str, state: &NodeState, sdp: &str) -> anyhow::Result<String> {
    #[derive(Serialize)]
    struct Offer<'a> {
        sdp: &'a str,
    }
    #[derive(Deserialize)]
    struct Answer {
        sdp: String,
    }
    let path = "/api/v1/node/connect";
    let body = serde_json::to_vec(&Offer { sdp })?;
    let auth = sign_node_request(state, "POST", path, &body)?;
    let mut request = reqwest::Client::new()
        .post(format!("{}{}", server.trim_end_matches('/'), path))
        .header("content-type", "application/json")
        .body(body);
    for (name, value) in auth.headers() {
        request = request.header(name, value);
    }
    let response = request.send().await?;
    if !response.status().is_success() {
        anyhow::bail!(
            "Node WebRTC bootstrap failed: {} {}",
            response.status(),
            response.text().await.unwrap_or_default()
        );
    }
    Ok(response.json::<Answer>().await?.sdp)
}
