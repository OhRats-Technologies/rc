mod auth;
mod direct;
mod hosted;
mod image;
mod lifecycle;
mod process;
mod schedule;
mod state;
mod webrtc;

pub use state::ControlManager;
use state::{ControlInner, ControlSession, PendingStart, SessionAuthority};

use crate::{
    ExecutionManager, MeshAuthority, NodeState, ProcessPolicy, ScheduleManager, TransportPolicy,
    bootstrap_lock, sync_lock,
};
use parking_lot::Mutex;
use rc_protocol::{ControlProof, NodeToServer, ServerToNode};
use std::{collections::HashMap, path::PathBuf, sync::Arc};
use tokio::sync::mpsc;

const CONTROL_PLAINTEXT_LIMIT: usize = 1_048_576;
const CONTROL_CIPHERTEXT_LIMIT: usize = 1_500_000;
impl ControlManager {
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        state: NodeState,
        state_dir: PathBuf,
        processes: Arc<dyn ExecutionManager>,
        outbound: mpsc::UnboundedSender<NodeToServer>,
        version: impl Into<String>,
        process_policy: Arc<dyn ProcessPolicy>,
        transport_policy: Arc<dyn TransportPolicy>,
        schedules: Arc<dyn ScheduleManager>,
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
            schedules,
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
                        self.reconcile_execution_authority();
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
                mode,
                cwd,
                environment,
                max_runtime_seconds,
                mcp_grant,
                mcp_signature,
                control_grant,
                credential_id,
                control_assertion,
            } => self.mcp_start(
                process_id,
                user_id,
                mode,
                cwd,
                environment,
                max_runtime_seconds,
                mcp_grant,
                mcp_signature,
                control_grant,
                credential_id,
                control_assertion,
            ),
            ServerToNode::McpExecutionInput {
                request_id,
                process_id,
                user_id,
                data,
                eof,
                mcp_grant,
                mcp_signature,
                control_grant,
                credential_id,
                control_assertion,
            } => self.mcp_input(
                request_id,
                process_id,
                user_id,
                data,
                eof,
                mcp_grant,
                mcp_signature,
                control_grant,
                credential_id,
                control_assertion,
            ),
            ServerToNode::McpExecutionSignal {
                request_id,
                process_id,
                user_id,
                signal,
                mcp_grant,
                mcp_signature,
                control_grant,
                credential_id,
                control_assertion,
            } => self.mcp_signal(
                request_id,
                process_id,
                user_id,
                signal,
                mcp_grant,
                mcp_signature,
                control_grant,
                credential_id,
                control_assertion,
            ),
            ServerToNode::McpExecutionStatus {
                request_id,
                process_id,
                user_id,
                cursor,
                wait_seconds,
                mcp_grant,
                mcp_signature,
                control_grant,
                credential_id,
                control_assertion,
            } => self.mcp_status(
                request_id,
                process_id,
                user_id,
                cursor,
                wait_seconds,
                mcp_grant,
                mcp_signature,
                control_grant,
                credential_id,
                control_assertion,
            ),
            ServerToNode::McpImageView {
                request_id,
                user_id,
                path,
                mcp_grant,
                mcp_signature,
                control_grant,
                credential_id,
                control_assertion,
            } => {
                self.mcp_image_view(
                    request_id,
                    user_id,
                    path,
                    mcp_grant,
                    mcp_signature,
                    control_grant,
                    credential_id,
                    control_assertion,
                )
                .await
            }
            ServerToNode::Update => self.handle_update().await,
        }
    }
}
