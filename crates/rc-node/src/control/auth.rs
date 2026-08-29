use super::{ControlManager, ControlSession, SessionAuthority};
use crate::{ApiControlAuthority, api_control_authority, load_lock, verify_control_proof};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rand::RngCore;
use rc_api_client::random_url_bytes;
use rc_crypto::{
    decode_x25519, derive_node_key, ready_payload, session_payload, sign_ed25519_seed,
    verify_ed25519,
};
use rc_protocol::{
    AuthoritySnapshot, ControlProof, IceServer, NodeToServer, control_attempts_payload,
};
use std::time::{Duration, Instant};

impl ControlManager {
    pub(super) fn challenge(&self, request_id: String) {
        let now = Instant::now();
        let challenge = random_url_bytes(32);
        let mut challenges = self.0.challenges.lock();
        challenges.retain(|_, expires| *expires > now);
        challenges.insert(challenge.clone(), now + Duration::from_secs(60));
        drop(challenges);
        self.emit(NodeToServer::ControlChallenge {
            request_id,
            challenge,
        });
    }

    #[allow(clippy::too_many_arguments)]
    pub(super) fn open(
        &self,
        request_id: String,
        challenge: String,
        server_user_id: String,
        client_id: String,
        grant: String,
        credential_id: String,
        assertion: String,
        client_public_key: String,
        signature: String,
        ice_servers: Vec<IceServer>,
    ) {
        if !self.consume_challenge(&challenge) {
            self.control_error(request_id, "control challenge expired");
            return;
        }
        let authority = match self.session_authority(&client_id, &grant, &credential_id, &assertion)
        {
            Ok(value) => value,
            Err(error) => {
                self.control_error(request_id, error);
                return;
            }
        };
        if authority.user_id != server_user_id {
            self.control_error(request_id, "control user mismatch");
            return;
        }
        if decode_x25519(&client_public_key).is_err() {
            self.control_error(request_id, "invalid control transport key");
            return;
        }
        let payload = session_payload(
            &challenge,
            &self.0.state.device_id,
            &client_id,
            &client_public_key,
        );
        if verify_ed25519(&authority.public_key, payload.as_bytes(), &signature).is_err() {
            self.control_error(request_id, "invalid control client signature");
            return;
        }
        let mut ephemeral = [0_u8; 32];
        rand::rngs::OsRng.fill_bytes(&mut ephemeral);
        let ephemeral_private = URL_SAFE_NO_PAD.encode(ephemeral);
        let ephemeral_public = match rc_crypto::x25519_public(&ephemeral_private) {
            Ok(value) => value,
            Err(error) => {
                self.control_error(request_id, error.to_string());
                return;
            }
        };
        let key = match derive_node_key(
            &self.0.state.transport_secret,
            &ephemeral_private,
            &client_public_key,
            &challenge,
            &self.0.state.device_id,
            &client_id,
        ) {
            Ok(value) => value,
            Err(error) => {
                self.control_error(request_id, error.to_string());
                return;
            }
        };
        let session_id = random_url_bytes(18);
        let transport_public_key = match self.0.state.transport_public_key() {
            Ok(value) => value,
            Err(error) => {
                self.control_error(request_id, error.to_string());
                return;
            }
        };
        let attempts = match self.0.transport_policy.attempts(ice_servers) {
            Ok(attempts) if !attempts.is_empty() => attempts,
            Ok(_) => {
                self.control_error(request_id, "transport policy returned no attempts");
                return;
            }
            Err(error) => {
                self.control_error(request_id, error);
                return;
            }
        };
        let ready = ready_payload(
            &challenge,
            &self.0.state.device_id,
            &client_id,
            &client_public_key,
            &transport_public_key,
            &ephemeral_public,
            &session_id,
            &control_attempts_payload(&attempts),
        );
        let node_signature = match sign_ed25519_seed(&self.0.state.identity_seed, ready.as_bytes())
        {
            Ok(value) => value,
            Err(error) => {
                self.control_error(request_id, error.to_string());
                return;
            }
        };
        self.0
            .sessions
            .lock()
            .insert(session_id.clone(), session_from_authority(key, authority));
        self.emit(NodeToServer::ControlReady {
            request_id,
            session_id,
            transport_public_key,
            ephemeral_public_key: ephemeral_public,
            signature: node_signature,
            attempts,
        });
    }

    fn session_authority(
        &self,
        client_id: &str,
        grant: &str,
        credential_id: &str,
        assertion: &str,
    ) -> Result<SessionAuthority, String> {
        if grant.is_empty() {
            return api_control_authority(&self.0.state_dir, client_id)
                .map(SessionAuthority::from)
                .map_err(|error| error.to_string());
        }
        let lock = load_lock(&self.0.state_dir).map_err(|error| error.to_string())?;
        let snapshot: AuthoritySnapshot =
            serde_json::from_str(&lock.snapshot).map_err(|_| "invalid RC Lock state".to_owned())?;
        let proof = ControlProof {
            grant: grant.to_owned(),
            credential_id: credential_id.to_owned(),
            assertion: assertion.to_owned(),
        };
        let authority = verify_control_proof(&snapshot, &proof, &lock.origin, &lock.rp_id)
            .map_err(|_| "control grant rejected".to_owned())?;
        if authority.grant.client_id != client_id {
            return Err("control grant rejected".into());
        }
        Ok(SessionAuthority {
            user_id: authority.grant.user_id,
            role: authority.role.clone(),
            public_key: authority.grant.signing_public_key,
            can_execute: authority.role != "viewer",
            can_manage_devices: authority.role == "owner",
        })
    }

    fn consume_challenge(&self, challenge: &str) -> bool {
        self.0
            .challenges
            .lock()
            .remove(challenge)
            .is_some_and(|expires| expires > Instant::now())
    }
}

fn session_from_authority(key: [u8; 32], authority: SessionAuthority) -> ControlSession {
    ControlSession {
        key,
        user_id: authority.user_id,
        role: authority.role,
        can_execute: authority.can_execute,
        can_manage_devices: authority.can_manage_devices,
        recv_sequence: 0,
        send_sequence: 0,
        transport_id: String::new(),
        peer: None,
        sender: None,
    }
}

impl From<ApiControlAuthority> for SessionAuthority {
    fn from(authority: ApiControlAuthority) -> Self {
        Self {
            user_id: authority.user_id,
            role: authority.role,
            public_key: authority.public_key,
            can_execute: authority.can_execute,
            can_manage_devices: authority.can_manage_devices,
        }
    }
}
