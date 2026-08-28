use super::ControlManager;
use crate::{ProcessEvent, lock_metadata};
use rc_protocol::{ControlMessage, NodeToServer};
use std::time::Duration;

impl ControlManager {
    pub fn send_process_event(&self, session_id: &str, event: ProcessEvent) -> bool {
        let message = match event {
            ProcessEvent::Started { id } => ControlMessage::ProcessStarted { id },
            ProcessEvent::Stdout { id, data } => ControlMessage::ProcessStdout { id, data },
            ProcessEvent::Stderr { id, data } => ControlMessage::ProcessStderr { id, data },
            ProcessEvent::Exit {
                id,
                exit_code,
                signal,
            } => ControlMessage::ProcessExit {
                id,
                exit_code: Some(exit_code),
                signal,
            },
        };
        self.send_frame(session_id, &message)
    }

    pub async fn close_all(&self) {
        let ids: Vec<_> = self.0.sessions.lock().keys().cloned().collect();
        for id in ids {
            self.close_session(&id).await;
        }
    }

    pub(super) async fn invalidate_sessions(&self) {
        let ids: Vec<_> = self.0.sessions.lock().keys().cloned().collect();
        for id in &ids {
            let _ = self.send_frame(id, &ControlMessage::Revoked);
        }
        for id in ids {
            self.close_session(&id).await;
        }
    }

    pub async fn shutdown(&self) {
        let sessions: Vec<_> = self
            .0
            .sessions
            .lock()
            .drain()
            .map(|(_, session)| session)
            .collect();
        for session in sessions {
            if let Some(peer) = session.peer {
                let _ = peer.close().await;
            }
        }
        self.0.pending_starts.lock().clear();
        for process_id in self.0.processes.relay_process_ids() {
            let _ = self.0.processes.signal(&process_id, "KILL");
        }
    }

    pub fn has_session(&self, session_id: &str) -> bool {
        self.0.sessions.lock().contains_key(session_id)
    }

    pub(super) async fn handle_update(&self) {
        match crate::replace_executable(&self.0.version).await {
            Ok(false) => self.emit(NodeToServer::UpdateResult {
                ok: true,
                version: self.0.version.clone(),
                error: String::new(),
            }),
            Ok(true) => {
                self.emit(NodeToServer::UpdateResult {
                    ok: true,
                    version: self.0.version.clone(),
                    error: String::new(),
                });
                tokio::time::sleep(Duration::from_millis(250)).await;
                self.0.processes.shutdown();
                let _ = crate::exec_current();
            }
            Err(error) => self.emit(NodeToServer::UpdateResult {
                ok: false,
                version: self.0.version.clone(),
                error: error.to_string(),
            }),
        }
    }

    pub(super) async fn close_session(&self, session_id: &str) {
        let session = self.0.sessions.lock().remove(session_id);
        let Some(session) = session else { return };
        if let Some(peer) = session.peer {
            let _ = peer.close().await;
        }
        self.0.processes.detach_secure_session(session_id);
        self.0
            .pending_starts
            .lock()
            .retain(|_, pending| pending.session_id != session_id);
        self.emit(NodeToServer::ControlClosed {
            session_id: session_id.to_owned(),
        });
    }

    pub(super) fn send_lock_state(&self) {
        let (hash, generation) = lock_metadata(&self.0.state_dir);
        self.emit(NodeToServer::LockState { hash, generation });
    }

    pub(super) fn control_error(&self, request_id: impl Into<String>, error: impl Into<String>) {
        self.emit(NodeToServer::ControlError {
            request_id: request_id.into(),
            error: error.into(),
        });
    }

    pub(super) fn emit(&self, message: NodeToServer) {
        let _ = self.0.outbound.send(message);
    }
}
