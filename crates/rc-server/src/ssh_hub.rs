use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use dashmap::DashMap;
use rc_protocol::NodeToServer;
use std::sync::Arc;
use tokio::sync::mpsc;

#[derive(Debug)]
pub enum SshRelay {
    Stdout(Vec<u8>),
    Stderr(Vec<u8>),
    Exit { code: i32, signal: String },
}

#[derive(Clone, Default)]
pub struct SshHub {
    sessions: Arc<DashMap<String, SshSession>>,
}

struct SshSession {
    device_id: String,
    process_id: String,
    sender: mpsc::UnboundedSender<SshRelay>,
}

impl SshHub {
    pub fn register(
        &self,
        session_id: &str,
        device_id: &str,
        process_id: &str,
    ) -> mpsc::UnboundedReceiver<SshRelay> {
        let (sender, receiver) = mpsc::unbounded_channel();
        self.sessions.insert(
            session_id.to_owned(),
            SshSession {
                device_id: device_id.to_owned(),
                process_id: process_id.to_owned(),
                sender,
            },
        );
        receiver
    }

    pub fn remove(&self, session_id: &str) -> Option<(String, String)> {
        self.sessions
            .remove(session_id)
            .map(|(_, session)| (session.device_id, session.process_id))
    }

    pub fn handle(&self, device_id: &str, message: &NodeToServer) -> bool {
        let (session_id, relay, terminal) = match message {
            NodeToServer::SshStdout { session_id, data } => {
                let Ok(bytes) = URL_SAFE_NO_PAD.decode(data) else {
                    return true;
                };
                (session_id, SshRelay::Stdout(bytes), false)
            }
            NodeToServer::SshStderr { session_id, data } => {
                let Ok(bytes) = URL_SAFE_NO_PAD.decode(data) else {
                    return true;
                };
                (session_id, SshRelay::Stderr(bytes), false)
            }
            NodeToServer::SshExit {
                session_id,
                exit_code,
                signal,
            } => (
                session_id,
                SshRelay::Exit {
                    code: *exit_code,
                    signal: signal.clone(),
                },
                true,
            ),
            _ => return false,
        };
        if let Some(session) = self.sessions.get(session_id)
            && session.device_id == device_id
        {
            let _ = session.sender.send(relay);
        }
        if terminal {
            self.sessions.remove(session_id);
        }
        true
    }

    pub fn release_device(&self, device_id: &str) {
        let ids: Vec<_> = self
            .sessions
            .iter()
            .filter(|entry| entry.value().device_id == device_id)
            .map(|entry| entry.key().clone())
            .collect();
        for id in ids {
            if let Some((_, session)) = self.sessions.remove(&id) {
                let _ = session.sender.send(SshRelay::Exit {
                    code: 255,
                    signal: "DISCONNECTED".into(),
                });
            }
        }
    }
}
