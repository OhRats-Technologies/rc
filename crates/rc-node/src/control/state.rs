use crate::{
    ExecutionManager, MeshAuthority, NodeState, ProcessEnvironment, ProcessExecutionMode,
    ProcessPolicy, ScheduleManager, TransportPolicy,
};
use parking_lot::Mutex;
use rc_protocol::{ControlTransportMessage, NodeToServer, TerminalSpec};
use std::{collections::HashMap, path::PathBuf, sync::Arc, time::Instant};
use tokio::sync::mpsc;
use webrtc::peer_connection::RTCPeerConnection;

#[derive(Clone)]
pub struct ControlManager(pub(super) Arc<ControlInner>);

pub(super) struct ControlInner {
    pub state: NodeState,
    pub version: String,
    pub state_dir: PathBuf,
    pub processes: Arc<dyn ExecutionManager>,
    pub outbound: mpsc::UnboundedSender<NodeToServer>,
    pub challenges: Mutex<HashMap<String, Instant>>,
    pub sessions: Mutex<HashMap<String, ControlSession>>,
    pub pending_starts: Mutex<HashMap<(String, String), PendingStart>>,
    pub mesh: Mutex<Option<Arc<MeshAuthority>>>,
    pub process_policy: Arc<dyn ProcessPolicy>,
    pub transport_policy: Arc<dyn TransportPolicy>,
    pub schedules: Arc<dyn ScheduleManager>,
}

pub(super) struct ControlSession {
    pub key: [u8; 32],
    pub user_id: String,
    pub role: String,
    pub can_execute: bool,
    pub can_manage_devices: bool,
    pub recv_sequence: u64,
    pub send_sequence: u64,
    pub transport_id: String,
    pub peer: Option<Arc<RTCPeerConnection>>,
    pub sender: Option<mpsc::UnboundedSender<ControlTransportMessage>>,
}

pub(super) struct PendingStart {
    pub session_id: String,
    pub user_id: String,
    pub principal: crate::ProcessPrincipal,
    pub mode: ProcessExecutionMode,
    pub environment: ProcessEnvironment,
    pub cwd: String,
    pub terminal: Option<TerminalSpec>,
    pub scrollback_bytes: u32,
    pub stdin_chunk_bytes: u32,
    pub terminate_grace_ms: u32,
    pub reattach_grace_ms: u32,
    pub max_runtime_ms: Option<u64>,
    pub expires: Instant,
}

pub(super) struct SessionAuthority {
    pub user_id: String,
    pub role: String,
    pub public_key: String,
    pub can_execute: bool,
    pub can_manage_devices: bool,
}

impl ControlManager {
    pub fn mesh_authority(&self) -> Option<Arc<MeshAuthority>> {
        self.0.mesh.lock().clone()
    }

    pub(super) fn refresh_mesh_authority(&self) {
        match MeshAuthority::from_lock(&self.0.state, &self.0.state_dir) {
            Ok(mesh) => *self.0.mesh.lock() = Some(Arc::new(mesh)),
            Err(error) => {
                eprintln!("RC mesh authority is unavailable: {error}");
                *self.0.mesh.lock() = None;
            }
        }
    }

    pub(super) fn reconcile_execution_authority(&self) {
        let Ok(lock) = crate::load_lock(&self.0.state_dir) else {
            return;
        };
        let Ok(snapshot) = serde_json::from_str::<rc_protocol::AuthoritySnapshot>(&lock.snapshot)
        else {
            return;
        };
        for id in self.0.processes.active_ids() {
            let Some((channel, authorization_id)) = self.0.processes.execution_authority(&id)
            else {
                continue;
            };
            if !execution_authorized(
                &snapshot,
                channel,
                &authorization_id,
                &self.0.state.device_id,
                crate::lock::now_ms(),
            ) {
                let _ = self.0.processes.signal(&id, "KILL");
            }
        }
    }
}

fn execution_authorized(
    snapshot: &rc_protocol::AuthoritySnapshot,
    channel: crate::ProcessChannel,
    authorization_id: &str,
    device_id: &str,
    now_ms: i64,
) -> bool {
    match channel {
        crate::ProcessChannel::Mcp => snapshot
            .mcp_grants
            .iter()
            .any(|grant| grant.id == authorization_id),
        crate::ProcessChannel::Schedule => snapshot.schedule_grants.iter().any(|grant| {
            grant.schedule_id == authorization_id
                && grant.device_id == device_id
                && (grant.expires_at == 0 || grant.expires_at > now_ms)
        }),
        crate::ProcessChannel::Control | crate::ProcessChannel::Ssh => true,
    }
}

#[cfg(test)]
mod tests {
    use super::execution_authorized;
    use crate::ProcessChannel;
    use rc_protocol::{AuthorityMcpGrant, AuthorityScheduleGrant, AuthoritySnapshot};

    fn snapshot() -> AuthoritySnapshot {
        AuthoritySnapshot {
            v: 1,
            workspace_id: "workspace".into(),
            devices: Vec::new(),
            members: Vec::new(),
            api_keys: Vec::new(),
            mcp_grants: vec![AuthorityMcpGrant {
                id: "mcp-live".into(),
                user_id: "user".into(),
                hash: "hash".into(),
            }],
            schedule_grants: vec![AuthorityScheduleGrant {
                schedule_id: "schedule-live".into(),
                device_id: "device".into(),
                user_id: "user".into(),
                spec_hash: "hash".into(),
                max_runtime_ms: 1_000,
                expires_at: 0,
            }],
        }
    }

    #[test]
    fn durable_execution_authority_is_channel_bound_and_revocable() {
        let snapshot = snapshot();
        assert!(execution_authorized(
            &snapshot,
            ProcessChannel::Mcp,
            "mcp-live",
            "device",
            1
        ));
        assert!(!execution_authorized(
            &snapshot,
            ProcessChannel::Mcp,
            "schedule-live",
            "device",
            1
        ));
        assert!(execution_authorized(
            &snapshot,
            ProcessChannel::Schedule,
            "schedule-live",
            "device",
            1
        ));
        assert!(!execution_authorized(
            &snapshot,
            ProcessChannel::Schedule,
            "revoked",
            "device",
            1
        ));
        assert!(!execution_authorized(
            &snapshot,
            ProcessChannel::Schedule,
            "schedule-live",
            "other-device",
            1
        ));
        let mut expired = snapshot;
        expired.schedule_grants[0].expires_at = 2;
        assert!(!execution_authorized(
            &expired,
            ProcessChannel::Schedule,
            "schedule-live",
            "device",
            2
        ));
    }
}
