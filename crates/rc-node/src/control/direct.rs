use super::{
    CONTROL_CIPHERTEXT_LIMIT, CONTROL_PLAINTEXT_LIMIT, ControlManager, process::principal,
};
use crate::{
    ProcessAction, ProcessChannel, ProcessEnvironment, ProcessExecutionMode, ProcessLifetime,
    ProcessResizeRequest, ProcessSignal, ProcessSignalRequest, ProcessStartRequest,
};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_crypto::{decrypt_frame, encrypt_frame};
use rc_protocol::{
    ControlMessage, ControlTransportMessage, EnvironmentBase, EnvironmentSpec, ExecutionMode,
    PROCESS_INPUT_CHUNK_LIMIT,
};
use std::time::Duration;

impl ControlManager {
    pub(super) fn receive_frame(
        &self,
        session_id: &str,
        sequence: u64,
        ciphertext: &str,
    ) -> anyhow::Result<()> {
        if sequence == 0 || ciphertext.is_empty() || ciphertext.len() > CONTROL_CIPHERTEXT_LIMIT {
            anyhow::bail!("invalid control frame");
        }
        let (plaintext, user_id, role, can_execute, can_manage_devices) = {
            let mut sessions = self.0.sessions.lock();
            let session = sessions
                .get_mut(session_id)
                .ok_or_else(|| anyhow::anyhow!("control session unavailable"))?;
            if sequence != session.recv_sequence + 1 {
                anyhow::bail!("invalid control frame sequence");
            }
            let plaintext =
                decrypt_frame(&session.key, 1, sequence, session_id, "c2n", ciphertext)?;
            if plaintext.len() > CONTROL_PLAINTEXT_LIMIT {
                anyhow::bail!("control message is too large");
            }
            session.recv_sequence = sequence;
            (
                plaintext,
                session.user_id.clone(),
                session.role.clone(),
                session.can_execute,
                session.can_manage_devices,
            )
        };
        let command: ControlMessage = serde_json::from_slice(&plaintext)?;
        self.handle_command(
            session_id,
            &user_id,
            &role,
            can_execute,
            can_manage_devices,
            command,
        )
    }

    fn handle_command(
        &self,
        session_id: &str,
        user_id: &str,
        role: &str,
        can_execute: bool,
        can_manage_devices: bool,
        command: ControlMessage,
    ) -> anyhow::Result<()> {
        let command = match command {
            value @ (ControlMessage::ScheduleList { .. }
            | ControlMessage::ScheduleUpsert { .. }
            | ControlMessage::ScheduleRemove { .. }
            | ControlMessage::ScheduleSetEnabled { .. }) => {
                return self.handle_schedule(session_id, user_id, can_manage_devices, value);
            }
            value => value,
        };
        match command {
            ControlMessage::NodeUpdate { request_id } => {
                if !can_manage_devices {
                    anyhow::bail!("owner required");
                }
                let manager = self.clone();
                let session_id = session_id.to_owned();
                tokio::spawn(async move {
                    match crate::replace_executable(&manager.0.version).await {
                        Ok(false) => {
                            let _ = manager.send_frame(
                                &session_id,
                                &ControlMessage::Result {
                                    request_id,
                                    output: "already up to date".into(),
                                },
                            );
                        }
                        Ok(true) => {
                            let _ = manager.send_frame(
                                &session_id,
                                &ControlMessage::Result {
                                    request_id,
                                    output: "ok".into(),
                                },
                            );
                            tokio::time::sleep(Duration::from_millis(250)).await;
                            manager.0.processes.shutdown();
                            let _ = crate::exec_current();
                        }
                        Err(error) => {
                            let _ = manager.send_frame(
                                &session_id,
                                &ControlMessage::Result {
                                    request_id,
                                    output: error.to_string(),
                                },
                            );
                        }
                    }
                });
            }
            ControlMessage::ProcessStart {
                id,
                mode,
                cwd,
                environment,
                terminal,
            } => {
                let principal = principal(user_id, role, can_execute, can_manage_devices);
                let plan = self
                    .0
                    .process_policy
                    .authorize_start(ProcessStartRequest {
                        execution_id: id.clone(),
                        mode: execution_mode(mode),
                        cwd,
                        environment: process_environment(environment),
                        terminal,
                        channel: ProcessChannel::Control,
                        lifetime: ProcessLifetime::Attached,
                        principal: principal.clone(),
                        max_runtime_ms: None,
                    })
                    .map_err(anyhow::Error::msg)?;
                self.queue_start(session_id, user_id, principal, id, plan);
            }
            ControlMessage::ProcessAttach { id } => {
                self.require_process_access(
                    &id,
                    ProcessAction::Attach,
                    principal(user_id, role, can_execute, can_manage_devices),
                )?;
                self.0.processes.attach_secure(&id, session_id);
            }
            ControlMessage::ProcessStdin { id, data } => {
                self.require_process_access(
                    &id,
                    ProcessAction::Input,
                    principal(user_id, role, can_execute, can_manage_devices),
                )?;
                self.require_writer(&id, session_id)?;
                let bytes = URL_SAFE_NO_PAD.decode(data)?;
                if bytes.len() > PROCESS_INPUT_CHUNK_LIMIT {
                    anyhow::bail!("process input too large");
                }
                self.0.processes.input(&id, &bytes)?;
            }
            ControlMessage::ProcessStdinClose { id } => {
                self.require_process_access(
                    &id,
                    ProcessAction::CloseInput,
                    principal(user_id, role, can_execute, can_manage_devices),
                )?;
                self.require_writer(&id, session_id)?;
                self.0.processes.close_input(&id);
            }
            ControlMessage::ProcessResize { id, cols, rows } => {
                let access = self.process_access(
                    &id,
                    ProcessAction::Resize,
                    principal(user_id, role, can_execute, can_manage_devices),
                )?;
                self.require_writer(&id, session_id)?;
                let size = self
                    .0
                    .process_policy
                    .normalize_resize(ProcessResizeRequest { access, cols, rows })
                    .map_err(anyhow::Error::msg)?;
                self.0.processes.resize(&id, size.cols, size.rows)?;
            }
            ControlMessage::ProcessSignal { id, signal } => {
                let access = self.process_access(
                    &id,
                    ProcessAction::Signal,
                    principal(user_id, role, can_execute, can_manage_devices),
                )?;
                self.require_writer(&id, session_id)?;
                let signal = self
                    .0
                    .process_policy
                    .authorize_signal(ProcessSignalRequest {
                        access,
                        signal: ProcessSignal::parse(&signal).map_err(anyhow::Error::msg)?,
                    })
                    .map_err(anyhow::Error::msg)?;
                self.0.processes.signal(&id, signal.legacy_name())?;
            }
            ControlMessage::ProcessStdout { .. }
            | ControlMessage::ProcessStderr { .. }
            | ControlMessage::ProcessStarted { .. }
            | ControlMessage::ProcessExit { .. }
            | ControlMessage::ScheduleList { .. }
            | ControlMessage::ScheduleUpsert { .. }
            | ControlMessage::ScheduleRemove { .. }
            | ControlMessage::ScheduleSetEnabled { .. }
            | ControlMessage::ScheduleResult { .. }
            | ControlMessage::Result { .. }
            | ControlMessage::Revoked => anyhow::bail!("invalid client control message"),
        }
        Ok(())
    }

    pub(super) fn send_frame(&self, session_id: &str, message: &ControlMessage) -> bool {
        let Ok(plaintext) = serde_json::to_vec(message) else {
            return false;
        };
        if plaintext.len() > CONTROL_PLAINTEXT_LIMIT {
            return false;
        };
        let (sender, sequence, ciphertext) = {
            let mut sessions = self.0.sessions.lock();
            let Some(session) = sessions.get_mut(session_id) else {
                return false;
            };
            let Some(sender) = session.sender.clone() else {
                return false;
            };
            session.send_sequence = match session.send_sequence.checked_add(1) {
                Some(value) => value,
                None => return false,
            };
            let sequence = session.send_sequence;
            let Ok(ciphertext) =
                encrypt_frame(&session.key, 2, sequence, session_id, "n2c", &plaintext)
            else {
                return false;
            };
            (sender, sequence, ciphertext)
        };
        sender
            .send(ControlTransportMessage::Frame {
                session_id: session_id.to_owned(),
                sequence,
                ciphertext,
            })
            .is_ok()
    }

    fn require_writer(&self, id: &str, session_id: &str) -> anyhow::Result<()> {
        if !self.0.processes.secure_writer(id, session_id) {
            anyhow::bail!("process attachment was superseded");
        }
        Ok(())
    }
}

pub(super) fn execution_mode(value: ExecutionMode) -> ProcessExecutionMode {
    match value {
        ExecutionMode::Argv { program, args } => ProcessExecutionMode::Argv { program, args },
        ExecutionMode::RcShell { script } => ProcessExecutionMode::RcShell { script },
        ExecutionMode::SystemShell { command } => ProcessExecutionMode::SystemShell { command },
        ExecutionMode::SystemLoginShell => ProcessExecutionMode::SystemLoginShell,
    }
}

pub(super) fn process_environment(value: EnvironmentSpec) -> ProcessEnvironment {
    ProcessEnvironment {
        base: match value.base {
            EnvironmentBase::Inherit => crate::ProcessEnvironmentBase::Inherit,
            EnvironmentBase::Clean => crate::ProcessEnvironmentBase::Clean,
        },
        changes: value
            .changes
            .into_iter()
            .map(|change| crate::ProcessEnvironmentChange {
                name: change.name,
                value: change.value,
            })
            .collect(),
    }
}
