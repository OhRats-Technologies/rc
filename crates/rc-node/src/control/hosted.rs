use super::ControlManager;
use crate::{
    ProcessAccessRequest, ProcessAction, ProcessChannel, ProcessEnvironment, ProcessExecutionMode,
    ProcessLifetime, ProcessPrincipal, ProcessResizeRequest, ProcessSignal, ProcessSignalRequest,
    ProcessSpec, ProcessStartRequest, hosted_control_authority,
};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_protocol::{ControlProof, NodeToServer, PROCESS_INPUT_CHUNK_LIMIT, TerminalSpec};

mod lifetime;
mod mcp;
pub(super) mod mcp_failures;
mod mcp_start;

impl ControlManager {
    #[allow(clippy::too_many_arguments)]
    pub(super) fn ssh_start(
        &self,
        process_id: String,
        session_id: String,
        user_id: String,
        command: String,
        cwd: String,
        terminal: Option<TerminalSpec>,
        grant: String,
        credential_id: String,
        assertion: String,
    ) {
        if session_id.is_empty() || session_id.len() > 100 {
            self.emit(NodeToServer::SshExit {
                session_id,
                exit_code: 126,
                signal: String::new(),
            });
            return;
        }
        let proof = ControlProof {
            grant,
            credential_id,
            assertion,
        };
        let Ok(authority) = hosted_control_authority(&self.0.state_dir, &proof, &user_id) else {
            self.emit(NodeToServer::SshExit {
                session_id,
                exit_code: 126,
                signal: String::new(),
            });
            return;
        };
        let principal = hosted_principal(&user_id, &authority.role);
        let plan = match self.0.process_policy.authorize_start(ProcessStartRequest {
            execution_id: process_id.clone(),
            mode: ProcessExecutionMode::SystemShell { command },
            cwd: (!cwd.is_empty()).then_some(cwd),
            environment: ProcessEnvironment::default(),
            terminal,
            channel: ProcessChannel::Ssh,
            lifetime: ProcessLifetime::Managed,
            principal: principal.clone(),
            max_runtime_ms: None,
        }) {
            Ok(value) => value,
            Err(_) => {
                self.emit(NodeToServer::SshExit {
                    session_id,
                    exit_code: 126,
                    signal: String::new(),
                });
                return;
            }
        };
        let spec = ProcessSpec {
            id: process_id.clone(),
            mode: plan.mode,
            environment: plan.environment,
            cwd: plan.cwd.unwrap_or_default(),
            terminal: plan.terminal,
            session_id: String::new(),
            user_id,
            authorization_id: String::new(),
            secure: false,
            relay_id: format!("ssh:{session_id}"),
            scrollback_bytes: plan.scrollback_bytes,
            stdin_chunk_bytes: plan.stdin_chunk_bytes,
            terminate_grace_ms: plan.terminate_grace_ms,
            reattach_grace_ms: plan.reattach_grace_ms,
            lifetime: ProcessLifetime::Managed,
            channel: ProcessChannel::Ssh,
            principal,
            max_runtime_ms: plan.max_runtime_ms,
        };
        if !matches!(self.0.processes.start(spec), Ok(true)) {
            self.emit(NodeToServer::SshExit {
                session_id,
                exit_code: 127,
                signal: String::new(),
            });
        }
    }

    pub(super) fn hosted_input(&self, relay_id: &str, data: &str, ssh: bool) {
        let Ok(bytes) = URL_SAFE_NO_PAD.decode(data) else {
            return;
        };
        if bytes.len() > PROCESS_INPUT_CHUNK_LIMIT {
            return;
        }
        if let Some((process_id, access)) = self.hosted_access(relay_id, ssh, ProcessAction::Input)
            && self.0.process_policy.authorize_access(access).is_ok()
        {
            let _ = self.0.processes.input(&process_id, &bytes);
        }
    }
    pub(super) fn hosted_close_input(&self, relay_id: &str, ssh: bool) {
        if let Some((process_id, access)) =
            self.hosted_access(relay_id, ssh, ProcessAction::CloseInput)
            && self.0.process_policy.authorize_access(access).is_ok()
        {
            self.0.processes.close_input(&process_id);
        }
    }
    pub(super) fn hosted_resize(&self, relay_id: &str, cols: u16, rows: u16) {
        if let Some((process_id, access)) =
            self.hosted_access(relay_id, true, ProcessAction::Resize)
            && let Ok(size) = self
                .0
                .process_policy
                .normalize_resize(ProcessResizeRequest { access, cols, rows })
        {
            let _ = self.0.processes.resize(&process_id, size.cols, size.rows);
        }
    }
    pub(super) fn hosted_signal(&self, relay_id: &str, signal: &str, ssh: bool) {
        if let Ok(requested) = ProcessSignal::parse(signal)
            && let Some((process_id, access)) =
                self.hosted_access(relay_id, ssh, ProcessAction::Signal)
            && let Ok(signal) = self
                .0
                .process_policy
                .authorize_signal(ProcessSignalRequest {
                    access,
                    signal: requested,
                })
        {
            let _ = self.0.processes.signal(&process_id, signal.legacy_name());
        }
    }
    fn hosted_process(&self, relay_id: &str, ssh: bool) -> Option<String> {
        self.0
            .processes
            .relay_process(&format!("{}:{relay_id}", if ssh { "ssh" } else { "mcp" }))
    }

    fn hosted_access(
        &self,
        relay_id: &str,
        ssh: bool,
        action: ProcessAction,
    ) -> Option<(String, ProcessAccessRequest)> {
        let process_id = self.hosted_process(relay_id, ssh)?;
        let owner = self.0.processes.owner(&process_id)?;
        let principal = ProcessPrincipal {
            user_id: owner.clone(),
            role: "operator".into(),
            can_execute: true,
            can_manage_devices: false,
        };
        Some((
            process_id.clone(),
            ProcessAccessRequest {
                execution_id: process_id,
                owner_user_id: owner,
                action,
                principal,
            },
        ))
    }
}

fn hosted_principal(user_id: &str, role: &str) -> ProcessPrincipal {
    ProcessPrincipal {
        user_id: user_id.to_owned(),
        role: role.to_owned(),
        can_execute: role != "viewer",
        can_manage_devices: role == "owner",
    }
}
