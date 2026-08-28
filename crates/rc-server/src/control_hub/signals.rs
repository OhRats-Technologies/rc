use super::{
    ControlHub, ControlReady, ControlReply, ControlSession, ControlSignalError, PendingKind,
};
use rc_protocol::{ControlProof, ServerToNode};
use uuid::Uuid;

impl ControlHub {
    pub async fn challenge(
        &self,
        user_id: &str,
        device_id: &str,
    ) -> Result<String, ControlSignalError> {
        let request_id = Uuid::new_v4().to_string();
        let reply = self
            .request(
                &request_id,
                super::pending::make(PendingKind::Challenge, device_id, user_id, "", Vec::new()),
                ServerToNode::ControlChallenge {
                    request_id: request_id.clone(),
                },
            )
            .await?;
        match reply {
            ControlReply::Challenge(value) => Ok(value),
            _ => Err(ControlSignalError::Protocol),
        }
    }

    #[allow(clippy::too_many_arguments)]
    pub async fn open(
        &self,
        user_id: &str,
        device_id: &str,
        client_id: &str,
        challenge: &str,
        public_key: &str,
        signature: &str,
        proof: Option<&ControlProof>,
    ) -> Result<ControlReady, ControlSignalError> {
        let ice_servers = self
            .inner
            .turn
            .ice_servers()
            .await
            .map_err(|_| ControlSignalError::Turn)?;
        let request_id = Uuid::new_v4().to_string();
        let (grant, credential_id, assertion) = proof
            .map(|proof| {
                (
                    proof.grant.clone(),
                    proof.credential_id.clone(),
                    proof.assertion.clone(),
                )
            })
            .unwrap_or_default();
        let reply = self
            .request(
                &request_id,
                super::pending::make(
                    PendingKind::Open,
                    device_id,
                    user_id,
                    client_id,
                    ice_servers.clone(),
                ),
                ServerToNode::ControlOpen {
                    request_id: request_id.clone(),
                    challenge: challenge.to_owned(),
                    user_id: user_id.to_owned(),
                    client_id: client_id.to_owned(),
                    grant,
                    credential_id,
                    assertion,
                    public_key: public_key.to_owned(),
                    signature: signature.to_owned(),
                },
            )
            .await?;
        match reply {
            ControlReply::Ready(value) => Ok(value),
            _ => Err(ControlSignalError::Protocol),
        }
    }

    pub async fn webrtc(
        &self,
        user_id: &str,
        client_id: Option<&str>,
        session_id: &str,
        device_id: &str,
        sdp: &str,
        relay_only: bool,
    ) -> Result<String, ControlSignalError> {
        let session = self
            .inner
            .sessions
            .get(session_id)
            .map(|value| value.clone())
            .ok_or(ControlSignalError::Unavailable)?;
        if session.user_id != user_id
            || client_id.is_some_and(|client_id| session.client_id != client_id)
            || session.device_id != device_id
        {
            return Err(ControlSignalError::Unavailable);
        }
        let ice_servers = if relay_only {
            session.ice_servers.clone()
        } else {
            direct_ice_servers(&session.ice_servers)
        };
        let request_id = Uuid::new_v4().to_string();
        let reply = self
            .request(
                &request_id,
                super::pending::make(
                    PendingKind::WebRtc,
                    device_id,
                    user_id,
                    &session.client_id,
                    Vec::new(),
                ),
                ServerToNode::ControlWebrtcOffer {
                    request_id: request_id.clone(),
                    session_id: session_id.to_owned(),
                    sdp: sdp.to_owned(),
                    ice_servers,
                    relay_only,
                },
            )
            .await?;
        match reply {
            ControlReply::WebRtc(value) => Ok(value),
            _ => Err(ControlSignalError::Protocol),
        }
    }

    pub async fn close(&self, user_id: &str, client_id: Option<&str>, session_id: &str) {
        let Some((_, session)) = self.inner.sessions.remove(session_id) else {
            return;
        };
        if session.user_id != user_id
            || client_id.is_some_and(|client_id| session.client_id != client_id)
        {
            self.inner.sessions.insert(session_id.to_owned(), session);
            return;
        }
        let _ = self
            .inner
            .nodes
            .send(
                &session.device_id,
                &ServerToNode::ControlClose {
                    session_id: session_id.to_owned(),
                },
            )
            .await;
    }

    pub(super) fn insert_session(&self, session_id: String, session: ControlSession) {
        self.inner.sessions.insert(session_id, session);
    }
}

fn direct_ice_servers(servers: &[rc_protocol::IceServer]) -> Vec<rc_protocol::IceServer> {
    servers
        .iter()
        .filter_map(|server| {
            let urls = server
                .urls
                .iter()
                .filter(|url| !url.to_ascii_lowercase().starts_with("turn:"))
                .filter(|url| !url.to_ascii_lowercase().starts_with("turns:"))
                .cloned()
                .collect::<Vec<_>>();
            (!urls.is_empty()).then(|| rc_protocol::IceServer {
                urls,
                username: server.username.clone(),
                credential: server.credential.clone(),
            })
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::direct_ice_servers;
    use rc_protocol::IceServer;

    #[test]
    fn direct_signaling_removes_turn_urls_without_dropping_stun() {
        let servers = vec![IceServer {
            urls: vec![
                "stun:stun.cloudflare.com:3478".into(),
                "turn:turn.cloudflare.com:3478?transport=udp".into(),
                "turns:turn.cloudflare.com:5349?transport=tcp".into(),
            ],
            username: "temporary".into(),
            credential: "secret".into(),
        }];
        assert_eq!(
            direct_ice_servers(&servers),
            vec![IceServer {
                urls: vec!["stun:stun.cloudflare.com:3478".into()],
                username: "temporary".into(),
                credential: "secret".into(),
            }]
        );
    }
}
