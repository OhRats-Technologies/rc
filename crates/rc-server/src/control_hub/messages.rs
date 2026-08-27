use super::{
    ControlHub, ControlReady, ControlReply, ControlSession, ControlSignalError, PendingKind,
};
use rc_protocol::NodeToServer;

impl ControlHub {
    pub fn handle_node_message(&self, device_id: &str, message: &NodeToServer) -> bool {
        match message {
            NodeToServer::ControlChallenge {
                request_id,
                challenge,
            } => self.finish(
                request_id,
                device_id,
                PendingKind::Challenge,
                Ok(ControlReply::Challenge(challenge.clone())),
            ),
            NodeToServer::ControlReady {
                request_id,
                session_id,
                transport_public_key,
                ephemeral_public_key,
                signature,
            } => {
                let Some(pending) = self.pending(request_id, device_id, PendingKind::Open) else {
                    return true;
                };
                if session_id.is_empty() || session_id.len() > 100 {
                    self.complete(request_id, pending, Err(ControlSignalError::Protocol));
                    return true;
                }
                let ready = ControlReady {
                    session_id: session_id.clone(),
                    transport_public_key: transport_public_key.clone(),
                    ephemeral_public_key: ephemeral_public_key.clone(),
                    signature: signature.clone(),
                    ice_servers: pending.ice_servers.clone(),
                };
                self.insert_session(
                    session_id.clone(),
                    ControlSession {
                        user_id: pending.user_id.clone(),
                        client_id: pending.client_id.clone(),
                        device_id: device_id.to_owned(),
                        ice_servers: pending.ice_servers.clone(),
                    },
                );
                self.complete(request_id, pending, Ok(ControlReply::Ready(ready)));
                true
            }
            NodeToServer::ControlWebrtcAnswer {
                request_id, sdp, ..
            } => self.finish(
                request_id,
                device_id,
                PendingKind::WebRtc,
                Ok(ControlReply::WebRtc(sdp.clone())),
            ),
            NodeToServer::ControlError { request_id, error } => {
                let Some(pending) = self.pending_any(request_id, device_id) else {
                    return true;
                };
                self.complete(
                    request_id,
                    pending,
                    Err(ControlSignalError::Rejected(error.clone())),
                );
                true
            }
            NodeToServer::ControlClosed { session_id } => {
                let remove = self
                    .inner
                    .sessions
                    .get(session_id)
                    .is_some_and(|session| session.device_id == device_id);
                if remove {
                    self.inner.sessions.remove(session_id);
                }
                true
            }
            _ => false,
        }
    }

    pub fn release_device(&self, device_id: &str) {
        let requests: Vec<_> = self
            .inner
            .pending
            .iter()
            .filter(|entry| entry.device_id == device_id)
            .map(|entry| entry.key().clone())
            .collect();
        for request_id in requests {
            if let Some((_, pending)) = self.inner.pending.remove(&request_id) {
                super::pending::send(&pending, Err(ControlSignalError::Disconnected));
            }
        }
        let sessions: Vec<_> = self
            .inner
            .sessions
            .iter()
            .filter(|entry| entry.device_id == device_id)
            .map(|entry| entry.key().clone())
            .collect();
        for session_id in sessions {
            self.inner.sessions.remove(&session_id);
        }
    }

    pub fn has_session(&self, session_id: &str) -> bool {
        self.inner.sessions.contains_key(session_id)
    }
}
