use crate::ohrats::rc_api_credentials::types::Request;
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use ed25519_dalek::{Signer, SigningKey};
use sha2::{Digest, Sha256};
use std::time::{SystemTime, UNIX_EPOCH};

pub fn signed(
    seed: u8,
    id: &str,
    at_ms: u64,
    nonce: &str,
    method: &str,
    path: &str,
    body: &[u8],
) -> Request {
    let timestamp = (at_ms / 1_000).to_string();
    let body_hash = Sha256::digest(body);
    let digest: String = body_hash.iter().map(|byte| format!("{byte:02x}")).collect();
    let payload = format!("rc-api-v1\n{id}\n{timestamp}\n{nonce}\n{method}\n{path}\n{digest}");
    let signature = SigningKey::from_bytes(&[seed; 32]).sign(payload.as_bytes());
    Request {
        key_id: id.into(),
        timestamp_seconds: timestamp,
        nonce: nonce.into(),
        method: method.into(),
        path_and_raw_query: path.into(),
        body: body.to_vec(),
        signature: URL_SAFE_NO_PAD.encode(signature.to_bytes()),
    }
}

pub fn key(seed: u8) -> String {
    URL_SAFE_NO_PAD.encode(
        SigningKey::from_bytes(&[seed; 32])
            .verifying_key()
            .to_bytes(),
    )
}

pub fn now_ms() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_or(0, |duration| duration.as_millis() as u64)
}
