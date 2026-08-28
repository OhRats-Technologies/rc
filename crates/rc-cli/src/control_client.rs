mod pins;
mod rtc;

use anyhow::{Result, bail};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use bytes::Bytes;
use ed25519_dalek::Signer;
use rand::RngCore;
use rc_api_client::{ApiClient, ControlChallenge, ControlReady, Credential, Device};
use rc_crypto::{
    decrypt_frame, derive_client_key, encrypt_frame, ready_payload, session_payload,
    verify_ed25519, x25519_public,
};
use rc_mesh::EncryptedFrameTransport;
use rc_protocol::{ControlMessage, ControlTransportMessage};
use serde::Serialize;
use std::{
    path::Path,
    sync::{
        Arc,
        atomic::{AtomicU64, Ordering},
    },
};
use tokio::sync::mpsc;
use webrtc::peer_connection::RTCPeerConnection;

const FRAME_LIMIT: usize = 1_500_000;
const PLAINTEXT_LIMIT: usize = 1_048_576;

pub struct RemoteControl {
    pub sender: ControlSender,
    pub receiver: ControlReceiver,
    peer: Arc<RTCPeerConnection>,
    api: ApiClient,
    session_id: String,
}

#[derive(Clone)]
pub struct ControlSender {
    session_id: String,
    key: [u8; 32],
    sequence: Arc<AtomicU64>,
    transport: Arc<dyn EncryptedFrameTransport>,
}

pub struct ControlReceiver {
    session_id: String,
    key: [u8; 32],
    sequence: u64,
    incoming: mpsc::Receiver<ControlTransportMessage>,
}

impl RemoteControl {
    pub async fn open(
        api: ApiClient,
        credential: &Credential,
        device: &Device,
        state_dir: &Path,
    ) -> Result<Self> {
        if !device.supports("webrtc") {
            bail!("RC Node does not support WebRTC control");
        }
        if device.identity_public_key.is_empty() || device.transport_public_key.is_empty() {
            bail!("RC Node cryptographic identity is unavailable");
        }
        pins::verify_device_pin(state_dir, device)?;
        let (client_id, signing) = match credential {
            Credential::Pop(key) => (key.id.clone(), key.signing_key().clone()),
            Credential::Bearer(_) => bail!("encrypted control requires a PoP signing key"),
        };
        #[derive(Serialize)]
        #[serde(rename_all = "camelCase")]
        struct Challenge<'a> {
            device_id: &'a str,
        }
        let challenge: ControlChallenge = api
            .post(
                "/api/v1/control/challenge",
                &Challenge {
                    device_id: &device.id,
                },
            )
            .await?;
        if challenge.challenge.is_empty() {
            bail!("missing Node challenge");
        }
        let mut private = [0u8; 32];
        rand::rngs::OsRng.fill_bytes(&mut private);
        let private = URL_SAFE_NO_PAD.encode(private);
        let public = x25519_public(&private)?;
        let signature = URL_SAFE_NO_PAD.encode(
            signing
                .sign(
                    session_payload(&challenge.challenge, &device.id, &client_id, &public)
                        .as_bytes(),
                )
                .to_bytes(),
        );
        #[derive(Serialize)]
        #[serde(rename_all = "camelCase")]
        struct Open<'a> {
            device_id: &'a str,
            challenge: &'a str,
            client_id: &'a str,
            public_key: &'a str,
            signature: &'a str,
        }
        let ready: ControlReady = api
            .post(
                "/api/v1/control/open",
                &Open {
                    device_id: &device.id,
                    challenge: &challenge.challenge,
                    client_id: &client_id,
                    public_key: &public,
                    signature: &signature,
                },
            )
            .await?;
        if ready.session_id.is_empty()
            || ready.ephemeral_public_key.is_empty()
            || ready.transport_public_key != device.transport_public_key
        {
            bail!("RC Node transport identity changed");
        }
        let payload = ready_payload(
            &challenge.challenge,
            &device.id,
            &client_id,
            &public,
            &ready.transport_public_key,
            &ready.ephemeral_public_key,
            &ready.session_id,
        );
        verify_ed25519(
            &device.identity_public_key,
            payload.as_bytes(),
            &ready.signature,
        )
        .map_err(|_| anyhow::anyhow!("RC Node handshake signature failed"))?;
        let key = derive_client_key(
            &private,
            &ready.transport_public_key,
            &ready.ephemeral_public_key,
            &challenge.challenge,
            &device.id,
            &client_id,
        )?;
        let (peer, transport, incoming) =
            rtc::open_webrtc(&api, &device.id, &ready.session_id, &ready.ice_servers).await?;
        Ok(Self {
            sender: ControlSender {
                session_id: ready.session_id.clone(),
                key,
                sequence: Arc::new(AtomicU64::new(0)),
                transport,
            },
            receiver: ControlReceiver {
                session_id: ready.session_id.clone(),
                key,
                sequence: 0,
                incoming,
            },
            peer,
            api,
            session_id: ready.session_id,
        })
    }

    pub async fn close(self) {
        let _ = self.peer.close().await;
        let _ = self
            .api
            .request_empty(
                reqwest::Method::DELETE,
                &format!("/api/v1/control/{}", encode_path(&self.session_id)),
            )
            .await;
    }
}

impl ControlSender {
    pub async fn send(&self, message: &ControlMessage) -> Result<()> {
        let plaintext = serde_json::to_vec(message)?;
        if plaintext.len() > PLAINTEXT_LIMIT {
            bail!("control message is too large");
        }
        let sequence = self
            .sequence
            .fetch_add(1, Ordering::SeqCst)
            .checked_add(1)
            .ok_or_else(|| anyhow::anyhow!("control sequence exhausted"))?;
        let ciphertext =
            encrypt_frame(&self.key, 1, sequence, &self.session_id, "c2n", &plaintext)?;
        let frame = ControlTransportMessage::Frame {
            session_id: self.session_id.clone(),
            sequence,
            ciphertext,
        };
        self.transport
            .send(Bytes::from(serde_json::to_vec(&frame)?))
            .await?;
        Ok(())
    }
}

impl ControlReceiver {
    pub async fn recv(&mut self) -> Result<ControlMessage> {
        let frame = self
            .incoming
            .recv()
            .await
            .ok_or_else(|| anyhow::anyhow!("WebRTC control closed"))?;
        let ControlTransportMessage::Frame {
            session_id,
            sequence,
            ciphertext,
        } = frame;
        if session_id != self.session_id
            || sequence != self.sequence + 1
            || ciphertext.is_empty()
            || ciphertext.len() > FRAME_LIMIT
        {
            bail!("invalid encrypted frame sequence");
        }
        let plaintext = decrypt_frame(&self.key, 2, sequence, &self.session_id, "n2c", &ciphertext)
            .map_err(|_| anyhow::anyhow!("encrypted frame authentication failed"))?;
        if plaintext.len() > PLAINTEXT_LIMIT {
            bail!("control message is too large");
        }
        self.sequence = sequence;
        let message: ControlMessage = serde_json::from_slice(&plaintext)?;
        if matches!(message, ControlMessage::Revoked) {
            bail!("control authorization changed; reopen the session");
        }
        Ok(message)
    }
}

pub(super) fn encode_path(value: &str) -> String {
    url::form_urlencoded::byte_serialize(value.as_bytes()).collect()
}
