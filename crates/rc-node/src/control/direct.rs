use super::{
    CONTROL_CIPHERTEXT_LIMIT, CONTROL_PLAINTEXT_LIMIT, ControlManager, PENDING_START_TTL,
    PendingStart, validate_start,
};
use crate::ProcessSpec;
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_crypto::{decrypt_frame, encrypt_frame};
use rc_protocol::{
    ControlMessage, ControlTransportMessage, NodeToServer, PROCESS_INPUT_CHUNK_LIMIT, TerminalSpec,
};
use std::time::{Duration, Instant};

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
                command,
                cwd,
                terminal,
            } => {
                if !can_execute {
                    anyhow::bail!("execute scope required");
                }
                validate_start(&id, &command, &cwd, terminal.as_ref())?;
                self.queue_start(session_id, user_id, id, command, cwd, terminal);
            }
            ControlMessage::ProcessAttach { id } => {
                self.require_process_access(&id, user_id, role, can_execute)?;
                self.0.processes.attach_secure(&id, session_id);
            }
            ControlMessage::ProcessStdin { id, data } => {
                self.require_process_access(&id, user_id, role, can_execute)?;
                let bytes = URL_SAFE_NO_PAD.decode(data)?;
                if bytes.len() > PROCESS_INPUT_CHUNK_LIMIT {
                    anyhow::bail!("process input too large");
                }
                self.0.processes.input(&id, &bytes)?;
            }
            ControlMessage::ProcessStdinClose { id } => {
                self.require_process_access(&id, user_id, role, can_execute)?;
                self.0.processes.close_input(&id);
            }
            ControlMessage::ProcessResize { id, cols, rows } => {
                self.require_process_access(&id, user_id, role, can_execute)?;
                if !(2..=500).contains(&cols) || !(2..=500).contains(&rows) {
                    anyhow::bail!("invalid terminal size");
                }
                self.0.processes.resize(&id, cols, rows)?;
            }
            ControlMessage::ProcessSignal { id, signal } => {
                self.require_process_access(&id, user_id, role, can_execute)?;
                if signal.is_empty() || signal.len() > 32 {
                    anyhow::bail!("invalid process signal");
                }
                self.0.processes.signal(&id, &signal)?;
            }
            ControlMessage::ProcessStdout { .. }
            | ControlMessage::ProcessStderr { .. }
            | ControlMessage::ProcessStarted { .. }
            | ControlMessage::ProcessExit { .. }
            | ControlMessage::Result { .. }
            | ControlMessage::Revoked => anyhow::bail!("invalid client control message"),
        }
        Ok(())
    }

    fn queue_start(
        &self,
        session_id: &str,
        user_id: &str,
        id: String,
        command: String,
        cwd: String,
        terminal: Option<TerminalSpec>,
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

    fn require_process_access(
        &self,
        id: &str,
        user_id: &str,
        role: &str,
        can_execute: bool,
    ) -> anyhow::Result<()> {
        if !can_execute {
            anyhow::bail!("execute scope required");
        }
        let owner = self
            .0
            .processes
            .owner(id)
            .ok_or_else(|| anyhow::anyhow!("process unavailable"))?;
        if role != "owner" && owner != user_id {
            anyhow::bail!("process access denied");
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
}
