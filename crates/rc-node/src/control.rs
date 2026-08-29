mod auth;
mod direct;
mod hosted;
mod lifecycle;
mod process;
mod webrtc;

use crate::{
    MeshAuthority, NodeState, ProcessManager, ProcessPolicy, TransportPolicy, bootstrap_lock,
    sync_lock,
};
use ::webrtc::peer_connection::RTCPeerConnection;
use parking_lot::Mutex;
use rc_protocol::{
    ControlProof, ControlTransportMessage, NodeToServer, ServerToNode, TerminalSpec,
};
use std::{collections::HashMap, path::PathBuf, sync::Arc, time::Instant};
use tokio::sync::mpsc;

const CONTROL_PLAINTEXT_LIMIT: usize = 1_048_576;
const CONTROL_CIPHERTEXT_LIMIT: usize = 1_500_000;

#[derive(Clone)]
pub struct ControlManager(Arc<ControlInner>);

struct ControlInner {
    state: NodeState,
    version: String,
    state_dir: PathBuf,
    processes: Arc<ProcessManager>,
    outbound: mpsc::UnboundedSender<NodeToServer>,
    challenges: Mutex<HashMap<String, Instant>>,
    sessions: Mutex<HashMap<String, ControlSession>>,
    pending_starts: Mutex<HashMap<(String, String), PendingStart>>,
    mesh: Mutex<Option<Arc<MeshAuthority>>>,
    process_policy: Arc<dyn ProcessPolicy>,
    transport_policy: Arc<dyn TransportPolicy>,
}

struct ControlSession {
    key: [u8; 32],
    user_id: String,
    role: String,
    can_execute: bool,
    can_manage_devices: bool,
    recv_sequence: u64,
    send_sequence: u64,
    transport_id: String,
    peer: Option<Arc<RTCPeerConnection>>,
    sender: Option<mpsc::UnboundedSender<ControlTransportMessage>>,
}

struct PendingStart {
    session_id: String,
    user_id: String,
    command: String,
    cwd: String,
    terminal: Option<TerminalSpec>,
    scrollback_bytes: u32,
    stdin_chunk_bytes: u32,
    terminate_grace_ms: u32,
    expires: Instant,
}

struct SessionAuthority {
    user_id: String,
    role: String,
    public_key: String,
    can_execute: bool,
    can_manage_devices: bool,
}

impl ControlManager {
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        state: NodeState,
        state_dir: PathBuf,
        processes: Arc<ProcessManager>,
        outbound: mpsc::UnboundedSender<NodeToServer>,
        version: impl Into<String>,
        process_policy: Arc<dyn ProcessPolicy>,
        transport_policy: Arc<dyn TransportPolicy>,
    ) -> Self {
        let mesh = MeshAuthority::from_lock(&state, &state_dir)
            .ok()
            .map(Arc::new);
        Self(Arc::new(ControlInner {
            state,
            version: version.into(),
            state_dir,
            processes,
            outbound,
            challenges: Mutex::new(HashMap::new()),
            sessions: Mutex::new(HashMap::new()),
            pending_starts: Mutex::new(HashMap::new()),
            mesh: Mutex::new(mesh),
            process_policy,
            transport_policy,
        }))
    }

    pub async fn handle(&self, server: &str, message: ServerToNode) {
        match message {
            ServerToNode::LockBootstrap { snapshot } => {
                match bootstrap_lock(&self.0.state_dir, &snapshot, server) {
                    Ok(()) => {
                        self.refresh_mesh_authority();
                        self.send_lock_state();
                    }
                    Err(error) => self.control_error("", error.to_string()),
                }
            }
            ServerToNode::LockSync {
                snapshot,
                previous_hash,
                previous_generation,
                grant,
                credential_id,
                assertion,
                signature,
            } => {
                let proof = ControlProof {
                    grant,
                    credential_id,
                    assertion,
                };
                match sync_lock(
                    &self.0.state_dir,
                    &snapshot,
                    &previous_hash,
                    previous_generation,
                    &proof,
                    &signature,
                ) {
                    Ok(()) => {
                        self.refresh_mesh_authority();
                        self.invalidate_sessions().await;
                        self.send_lock_state();
                    }
                    Err(error) => self.control_error("", error.to_string()),
                }
            }
            ServerToNode::ProcessPermit { id, user_id } => self.permit_start(&id, &user_id),
            ServerToNode::ControlChallenge { request_id } => self.challenge(request_id),
            ServerToNode::ControlOpen {
                request_id,
                challenge,
                user_id,
                client_id,
                grant,
                credential_id,
                assertion,
                public_key,
                signature,
                ice_servers,
            } => self.open(
                request_id,
                challenge,
                user_id,
                client_id,
                grant,
                credential_id,
                assertion,
                public_key,
                signature,
                ice_servers,
            ),
            ServerToNode::ControlWebrtcOffer {
                request_id,
                session_id,
                sdp,
                mode,
                ice_servers,
            } => {
                self.answer_webrtc(request_id, session_id, sdp, mode, ice_servers)
                    .await
            }
            ServerToNode::ControlClose { session_id } => self.close_session(&session_id).await,
            ServerToNode::SshStart {
                process_id,
                session_id,
                user_id,
                command,
                cwd,
                terminal,
                grant,
                credential_id,
                assertion,
            } => self.ssh_start(
                process_id,
                session_id,
                user_id,
                command,
                cwd,
                terminal,
                grant,
                credential_id,
                assertion,
            ),
            ServerToNode::SshStdin { session_id, data } => {
                self.hosted_input(&session_id, &data, true)
            }
            ServerToNode::SshStdinClose { session_id } => {
                self.hosted_close_input(&session_id, true)
            }
            ServerToNode::SshResize {
                session_id,
                cols,
                rows,
            } => self.hosted_resize(&session_id, cols, rows),
            ServerToNode::SshSignal { session_id, signal } => {
                self.hosted_signal(&session_id, &signal, true)
            }
            ServerToNode::McpStart {
                process_id,
                user_id,
                command,
                cwd,
                mcp_grant,
                mcp_signature,
                control_grant,
                credential_id,
                control_assertion,
            } => self.mcp_start(
                process_id,
                user_id,
                command,
                cwd,
                mcp_grant,
                mcp_signature,
                control_grant,
                credential_id,
                control_assertion,
            ),
            ServerToNode::McpStdin { process_id, data } => {
                self.hosted_input(&process_id, &data, false)
            }
            ServerToNode::McpStdinClose { process_id } => {
                self.hosted_close_input(&process_id, false)
            }
            ServerToNode::McpSignal { process_id, signal } => {
                self.hosted_signal(&process_id, &signal, false)
            }
            ServerToNode::Update => self.handle_update().await,
        }
    }

    pub fn mesh_authority(&self) -> Option<Arc<MeshAuthority>> {
        self.0.mesh.lock().clone()
    }

    fn refresh_mesh_authority(&self) {
        match MeshAuthority::from_lock(&self.0.state, &self.0.state_dir) {
            Ok(mesh) => {
                *self.0.mesh.lock() = Some(Arc::new(mesh));
            }
            Err(error) => {
                eprintln!("RC mesh authority is unavailable: {error}");
                *self.0.mesh.lock() = None;
            }
        }
    }
}
