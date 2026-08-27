use crate::CryptoError;
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use ed25519_dalek::{Signer, SigningKey};
use sha2::{Digest, Sha256};

pub fn api_payload(
    key_id: &str,
    timestamp: &str,
    nonce: &str,
    method: &str,
    request_uri: &str,
    body: &[u8],
) -> String {
    let digest = Sha256::digest(body);
    format!(
        "rc-api-v1\n{key_id}\n{timestamp}\n{nonce}\n{method}\n{request_uri}\n{}",
        hex_lower(&digest)
    )
}

pub fn sign_api_seed(
    seed: &str,
    key_id: &str,
    timestamp: &str,
    nonce: &str,
    method: &str,
    request_uri: &str,
    body: &[u8],
) -> Result<String, CryptoError> {
    let bytes: [u8; 32] = URL_SAFE_NO_PAD
        .decode(seed)?
        .try_into()
        .map_err(|_| CryptoError::KeyLength)?;
    let payload = api_payload(key_id, timestamp, nonce, method, request_uri, body);
    Ok(URL_SAFE_NO_PAD.encode(
        SigningKey::from_bytes(&bytes)
            .sign(payload.as_bytes())
            .to_bytes(),
    ))
}

fn hex_lower(bytes: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut out = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        out.push(HEX[(byte >> 4) as usize] as char);
        out.push(HEX[(byte & 15) as usize] as char);
    }
    out
}
