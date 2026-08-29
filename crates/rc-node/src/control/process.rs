use super::{ControlManager, PENDING_START_TTL, PendingStart};
use crate::{ProcessAccessRequest, ProcessAction, ProcessPrincipal, ProcessSpec};
use rc_protocol::{ControlMessage, NodeToServer, TerminalSpec};
use std::time::Instant;

impl ControlManager {
    pub(super) fn queue_start(
        &self,
        session_id: &str,
        user_id: &str,
        id: String,
        command: String,
        cwd: String,
        terminal: Option<TerminalSpec>,
        scrollback_bytes: u32,
        stdin_chunk_bytes: u32,
    ) {
        let now = Instant::now();
        let mut pending = self.0.pending_starts.lock();
        pending.retain(|_, value| value.expires > now);
        pending.insert(
            (id.clone(), user_id.to_owned()),
            PendingStart {
                session_id: session_id.to_owned(),
                user_id: user_id.to_owned(),
                command,
                cwd,
                terminal,
                scrollback_bytes,
                stdin_chunk_bytes,
                expires: now + PENDING_START_TTL,
            },
        );
        drop(pending);
        self.emit(NodeToServer::ProcessStartRequest {
            id,
            user_id: user_id.to_owned(),
        });
    }

    pub(super) fn permit_start(&self, id: &str, user_id: &str) {
        let pending = self
            .0
            .pending_starts
            .lock()
            .remove(&(id.to_owned(), user_id.to_owned()));
        let Some(pending) = pending else { return };
        if pending.expires <= Instant::now() || !self.has_session(&pending.session_id) {
            return;
        }
        let spec = ProcessSpec {
            id: id.to_owned(),
            command: pending.command,
            cwd: pending.cwd,
            terminal: pending.terminal,
            session_id: pending.session_id.clone(),
            user_id: pending.user_id,
            secure: true,
            relay_id: String::new(),
            scrollback_bytes: pending.scrollback_bytes,
            stdin_chunk_bytes: pending.stdin_chunk_bytes,
        };
        if self.0.processes.start(spec).is_err() {
            self.emit(NodeToServer::ProcessExit {
                id: id.to_owned(),
                exit_code: 127,
                signal: String::new(),
            });
            let _ = self.send_frame(
                &pending.session_id,
                &ControlMessage::ProcessExit {
                    id: id.to_owned(),
                    exit_code: Some(127),
                    signal: String::new(),
                },
            );
        }
    }

    pub(super) fn require_process_access(
        &self,
        id: &str,
        action: ProcessAction,
        principal: ProcessPrincipal,
    ) -> anyhow::Result<()> {
        let request = self.process_access(id, action, principal)?;
        self.0
            .process_policy
            .authorize_access(request)
            .map_err(anyhow::Error::msg)
    }

    pub(super) fn process_access(
        &self,
        id: &str,
        action: ProcessAction,
        principal: ProcessPrincipal,
    ) -> anyhow::Result<ProcessAccessRequest> {
        let owner = self
            .0
            .processes
            .owner(id)
            .ok_or_else(|| anyhow::anyhow!("process unavailable"))?;
        Ok(ProcessAccessRequest {
            process_id: id.to_owned(),
            owner_user_id: owner,
            action,
            principal,
        })
    }
}

pub(super) fn principal(
    user_id: &str,
    role: &str,
    can_execute: bool,
    can_manage_devices: bool,
) -> ProcessPrincipal {
    ProcessPrincipal {
        user_id: user_id.to_owned(),
        role: role.to_owned(),
        can_execute,
        can_manage_devices,
    }
}
