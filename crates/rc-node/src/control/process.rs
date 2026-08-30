use super::{ControlManager, PendingStart};
use crate::{ProcessAccessRequest, ProcessAction, ProcessPrincipal, ProcessSpec, ProcessStartPlan};
use rc_protocol::{ControlMessage, NodeToServer};
use std::time::Instant;

impl ControlManager {
    pub(super) fn queue_start(
        &self,
        session_id: &str,
        user_id: &str,
        principal: ProcessPrincipal,
        id: String,
        plan: ProcessStartPlan,
    ) {
        let now = Instant::now();
        let mut pending = self.0.pending_starts.lock();
        pending.retain(|_, value| value.expires > now);
        pending.insert(
            (id.clone(), user_id.to_owned()),
            PendingStart {
                session_id: session_id.to_owned(),
                user_id: user_id.to_owned(),
                principal,
                mode: plan.mode,
                environment: plan.environment,
                cwd: plan.cwd.unwrap_or_default(),
                terminal: plan.terminal,
                scrollback_bytes: plan.scrollback_bytes,
                stdin_chunk_bytes: plan.stdin_chunk_bytes,
                terminate_grace_ms: plan.terminate_grace_ms,
                reattach_grace_ms: plan.reattach_grace_ms,
                max_runtime_ms: plan.max_runtime_ms,
                expires: now
                    + std::time::Duration::from_millis(u64::from(plan.authorization_timeout_ms)),
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
            mode: pending.mode,
            environment: pending.environment,
            cwd: pending.cwd,
            terminal: pending.terminal,
            session_id: pending.session_id.clone(),
            user_id: pending.user_id,
            authorization_id: String::new(),
            secure: true,
            relay_id: String::new(),
            scrollback_bytes: pending.scrollback_bytes,
            stdin_chunk_bytes: pending.stdin_chunk_bytes,
            terminate_grace_ms: pending.terminate_grace_ms,
            reattach_grace_ms: pending.reattach_grace_ms,
            lifetime: crate::ProcessLifetime::Attached,
            channel: crate::ProcessChannel::Control,
            principal: pending.principal,
            max_runtime_ms: pending.max_runtime_ms,
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
            execution_id: id.to_owned(),
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
