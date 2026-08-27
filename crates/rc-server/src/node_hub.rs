use dashmap::DashMap;
use rc_protocol::{NodeToServer, ServerToNode};
use std::sync::Arc;
use tokio::sync::{RwLock, broadcast};
use webrtc::{data_channel::RTCDataChannel, peer_connection::RTCPeerConnection};

#[derive(Debug, Clone)]
pub struct NodeInbound {
    pub device_id: String,
    pub message: NodeToServer,
}

#[derive(Clone)]
pub struct NodeHub {
    peers: Arc<DashMap<String, Arc<NodePeer>>>,
    events: broadcast::Sender<NodeInbound>,
}

impl Default for NodeHub {
    fn default() -> Self {
        let (events, _) = broadcast::channel(1024);
        Self {
            peers: Arc::new(DashMap::new()),
            events,
        }
    }
}

pub struct NodePeer {
    connection_id: String,
    peer: Arc<RTCPeerConnection>,
    channel: RwLock<Option<Arc<RTCDataChannel>>>,
}

impl NodeHub {
    pub async fn insert_pending(
        &self,
        device_id: &str,
        connection_id: String,
        peer: Arc<RTCPeerConnection>,
    ) -> bool {
        let next = Arc::new(NodePeer {
            connection_id,
            peer,
            channel: RwLock::new(None),
        });
        let replaced_online = if let Some((_, old)) = self.peers.remove(device_id) {
            let online = old.channel.read().await.is_some();
            let _ = old.peer.close().await;
            online
        } else {
            false
        };
        self.peers.insert(device_id.to_owned(), next);
        replaced_online
    }

    pub async fn set_channel(
        &self,
        device_id: &str,
        connection_id: &str,
        channel: Arc<RTCDataChannel>,
    ) -> bool {
        let Some(peer) = self.peers.get(device_id).map(|value| value.clone()) else {
            return false;
        };
        if peer.connection_id != connection_id {
            return false;
        }
        let mut current = peer.channel.write().await;
        if current.is_some() {
            return false;
        }
        *current = Some(channel);
        true
    }

    pub async fn send(&self, device_id: &str, message: &ServerToNode) -> anyhow::Result<()> {
        let peer = self
            .peers
            .get(device_id)
            .map(|value| value.clone())
            .ok_or_else(|| anyhow::anyhow!("Node is offline"))?;
        let channel = peer
            .channel
            .read()
            .await
            .clone()
            .ok_or_else(|| anyhow::anyhow!("Node is connecting"))?;
        channel.send_text(serde_json::to_string(message)?).await?;
        Ok(())
    }

    pub async fn online(&self, device_id: &str) -> bool {
        let Some(peer) = self.peers.get(device_id).map(|value| value.clone()) else {
            return false;
        };
        peer.channel.read().await.is_some()
    }

    pub fn subscribe(&self) -> broadcast::Receiver<NodeInbound> {
        self.events.subscribe()
    }
    pub fn publish(&self, value: NodeInbound) {
        let _ = self.events.send(value);
    }

    pub async fn remove_if(&self, device_id: &str, connection_id: &str) -> bool {
        let matches = self
            .peers
            .get(device_id)
            .map(|peer| connection_id.is_empty() || peer.connection_id == connection_id)
            .unwrap_or(false);
        if matches && let Some((_, peer)) = self.peers.remove(device_id) {
            let _ = peer.peer.close().await;
            return true;
        }
        false
    }
}
