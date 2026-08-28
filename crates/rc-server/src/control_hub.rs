mod messages;
mod pending;
mod signals;

use crate::{NodeHub, TurnProvider};
use dashmap::DashMap;
use parking_lot::Mutex;
use rc_protocol::IceServer;
use std::sync::Arc;
use tokio::sync::oneshot;

#[derive(Clone)]
pub struct ControlHub {
    inner: Arc<Inner>,
}

struct Inner {
    nodes: NodeHub,
    turn: TurnProvider,
    pending: DashMap<String, Arc<Pending>>,
    sessions: DashMap<String, ControlSession>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum PendingKind {
    Challenge,
    Open,
    WebRtc,
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, serde::Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum ControlIceMode {
    Host,
    #[default]
    Stun,
    Relay,
}

struct Pending {
    kind: PendingKind,
    device_id: String,
    user_id: String,
    client_id: String,
    ice_servers: Vec<IceServer>,
    sender: Mutex<Option<oneshot::Sender<Result<ControlReply, ControlSignalError>>>>,
}

#[derive(Debug, Clone)]
struct ControlSession {
    user_id: String,
    client_id: String,
    device_id: String,
    ice_servers: Vec<IceServer>,
}

#[derive(Debug)]
enum ControlReply {
    Challenge(String),
    Ready(ControlReady),
    WebRtc(String),
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ControlReady {
    pub session_id: String,
    pub transport_public_key: String,
    pub ephemeral_public_key: String,
    pub signature: String,
    pub ice_servers: Vec<IceServer>,
}

#[derive(Debug, Clone, thiserror::Error, PartialEq, Eq)]
pub enum ControlSignalError {
    #[error("device is offline")]
    Offline,
    #[error("RC Node signaling timed out")]
    Timeout,
    #[error("device disconnected")]
    Disconnected,
    #[error("control session unavailable")]
    Unavailable,
    #[error("TURN unavailable")]
    Turn,
    #[error("control request rejected: {0}")]
    Rejected(String),
    #[error("invalid control signaling response")]
    Protocol,
}

impl ControlHub {
    pub fn new(nodes: NodeHub, turn: TurnProvider) -> Self {
        Self {
            inner: Arc::new(Inner {
                nodes,
                turn,
                pending: DashMap::new(),
                sessions: DashMap::new(),
            }),
        }
    }
}
