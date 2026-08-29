use crate::{ohrats::rc_api_credentials::types::Request, validate};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use ed25519_dalek::{Signature, Verifier, VerifyingKey};
use sha2::{Digest, Sha256};

pub fn verify(request: &Request, public_key: &str) -> Result<(), String> {
    validate::id(&request.key_id, "key id")?;
    validate::nonce(&request.nonce)?;
    validate::text(&request.timestamp_seconds, "timestamp", 32)?;
    validate::text(&request.method, "method", 16)?;
    if !request.path_and_raw_query.starts_with('/')
        || request.path_and_raw_query.len() > 4096
        || request.path_and_raw_query.chars().any(char::is_control)
    {
        return Err("invalid request path".into());
    }
    validate::public_key(public_key)?;
    validate::text(&request.signature, "signature", validate::MAX_SIGNATURE)?;
    let key = URL_SAFE_NO_PAD
        .decode(public_key)
        .map_err(|_| "invalid public key")?;
    let signature = URL_SAFE_NO_PAD
        .decode(&request.signature)
        .map_err(|_| "invalid signature")?;
    let key = VerifyingKey::from_bytes(&key.try_into().map_err(|_| "invalid public key")?)
        .map_err(|_| "invalid public key")?;
    let signature = Signature::from_bytes(&signature.try_into().map_err(|_| "invalid signature")?);
    key.verify(payload(request).as_bytes(), &signature)
        .map_err(|_| "invalid signature".into())
}

pub fn payload(request: &Request) -> String {
    let digest = Sha256::digest(&request.body);
    format!(
        "rc-api-v1\n{}\n{}\n{}\n{}\n{}\n{}",
        request.key_id,
        request.timestamp_seconds,
        request.nonce,
        request.method.to_ascii_uppercase(),
        request.path_and_raw_query,
        hex(&digest)
    )
}

pub fn hash(value: &str) -> Vec<u8> {
    Sha256::digest(value.as_bytes()).to_vec()
}

fn hex(bytes: &[u8]) -> String {
    bytes
        .iter()
        .flat_map(|byte| {
            [
                b"0123456789abcdef"[(byte >> 4) as usize] as char,
                b"0123456789abcdef"[(byte & 15) as usize] as char,
            ]
        })
        .collect()
}
