use super::ControlManager;
use super::validate_start;
use crate::{ProcessSpec, hosted_control_authority, verify_mcp_grant};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_protocol::{ControlProof, NodeToServer, TerminalSpec};

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
        if validate_start(&process_id, &command, &cwd, terminal.as_ref()).is_err()
            || session_id.is_empty()
            || session_id.len() > 100
        {
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
        if hosted_control_authority(&self.0.state_dir, &proof, &user_id).is_err() {
            self.emit(NodeToServer::SshExit {
                session_id,
                exit_code: 126,
                signal: String::new(),
            });
            return;
        }
        let spec = ProcessSpec {
            id: process_id.clone(),
            command,
            cwd,
            terminal,
            session_id: String::new(),
            user_id,
            secure: false,
            relay_id: format!("ssh:{session_id}"),
        };
        self.0
            .ssh_sessions
            .lock()
            .insert(session_id.clone(), process_id);
        if !matches!(self.0.processes.start(spec), Ok(true)) {
            self.0.ssh_sessions.lock().remove(&session_id);
            self.emit(NodeToServer::SshExit {
                session_id,
                exit_code: 127,
                signal: String::new(),
            });
        }
    }

    #[allow(clippy::too_many_arguments)]
    pub(super) fn mcp_start(
        &self,
        process_id: String,
        user_id: String,
        command: String,
        cwd: String,
        mcp_grant: String,
        mcp_signature: String,
        control_grant: String,
        credential_id: String,
        control_assertion: String,
    ) {
        if validate_start(&process_id, &command, &cwd, None).is_err() {
            self.emit(NodeToServer::McpExit {
                process_id,
                exit_code: 126,
                signal: String::new(),
            });
            return;
        }
        let proof = ControlProof {
            grant: control_grant,
            credential_id,
            assertion: control_assertion,
        };
        let Ok(authority) = hosted_control_authority(&self.0.state_dir, &proof, &user_id) else {
            self.emit(NodeToServer::McpExit {
                process_id,
                exit_code: 126,
                signal: String::new(),
            });
            return;
        };
        if verify_mcp_grant(
            &self.0.state_dir,
            &mcp_grant,
            &mcp_signature,
            &authority,
            &user_id,
            &self.0.state.device_id,
        )
        .is_err()
        {
            self.emit(NodeToServer::McpExit {
                process_id,
                exit_code: 126,
                signal: String::new(),
            });
            return;
        }
        let spec = ProcessSpec {
            id: process_id.clone(),
            command,
            cwd,
            terminal: None,
            session_id: String::new(),
            user_id,
            secure: false,
            relay_id: format!("mcp:{process_id}"),
        };
        self.0
            .mcp_processes
            .lock()
            .insert(process_id.clone(), process_id.clone());
        if !matches!(self.0.processes.start(spec), Ok(true)) {
            self.0.mcp_processes.lock().remove(&process_id);
            self.emit(NodeToServer::McpExit {
                process_id,
                exit_code: 127,
                signal: String::new(),
            });
        }
    }

    pub(super) fn hosted_input(&self, relay_id: &str, data: &str, ssh: bool) {
        let Ok(bytes) = URL_SAFE_NO_PAD.decode(data) else {
            return;
        };
        if bytes.len() > 131_072 {
            return;
        }
        if let Some(process_id) = self.hosted_process(relay_id, ssh) {
            let _ = self.0.processes.input(&process_id, &bytes);
        }
    }
    pub(super) fn hosted_close_input(&self, relay_id: &str, ssh: bool) {
        if let Some(process_id) = self.hosted_process(relay_id, ssh) {
            self.0.processes.close_input(&process_id);
        }
    }
    pub(super) fn hosted_resize(&self, relay_id: &str, cols: u16, rows: u16) {
        if let Some(process_id) = self.hosted_process(relay_id, true) {
            let _ = self.0.processes.resize(&process_id, cols, rows);
        }
    }
    pub(super) fn hosted_signal(&self, relay_id: &str, signal: &str, ssh: bool) {
        if signal.is_empty() || signal.len() > 32 {
            return;
        }
        if let Some(process_id) = self.hosted_process(relay_id, ssh) {
            let _ = self.0.processes.signal(&process_id, signal);
        }
    }
    fn hosted_process(&self, relay_id: &str, ssh: bool) -> Option<String> {
        if ssh {
            self.0.ssh_sessions.lock().get(relay_id).cloned()
        } else {
            self.0.mcp_processes.lock().get(relay_id).cloned()
        }
    }
}
