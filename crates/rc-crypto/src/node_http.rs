use crate::CryptoError;
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use ed25519_dalek::{Signature, Signer, SigningKey, Verifier, VerifyingKey};
use sha2::{Digest, Sha256};

pub fn node_http_payload(
    device_id: &str,
    timestamp: &str,
    nonce: &str,
    method: &str,
    path: &str,
    body: &[u8],
) -> String {
    let digest = Sha256::digest(body);
    format!(
        "rc-node-http-v1\n{device_id}\n{timestamp}\n{nonce}\n{method}\n{path}\n{}",
        hex_lower(&digest)
    )
}

pub fn sign_node_http(
    seed: &str,
    device_id: &str,
    timestamp: &str,
    nonce: &str,
    method: &str,
    path: &str,
    body: &[u8],
) -> Result<String, CryptoError> {
    let seed: [u8; 32] = URL_SAFE_NO_PAD
        .decode(seed)?
        .try_into()
        .map_err(|_| CryptoError::KeyLength)?;
    let payload = node_http_payload(device_id, timestamp, nonce, method, path, body);
    Ok(URL_SAFE_NO_PAD.encode(
        SigningKey::from_bytes(&seed)
            .sign(payload.as_bytes())
            .to_bytes(),
    ))
}

pub fn verify_node_http(
    public_key: &str,
    signature: &str,
    payload: &str,
) -> Result<(), CryptoError> {
    let public: [u8; 32] = URL_SAFE_NO_PAD
        .decode(public_key)?
        .try_into()
        .map_err(|_| CryptoError::KeyLength)?;
    let signature: [u8; 64] = URL_SAFE_NO_PAD
        .decode(signature)?
        .try_into()
        .map_err(|_| CryptoError::KeyLength)?;
    VerifyingKey::from_bytes(&public)
        .map_err(|_| CryptoError::Signature)?
        .verify(payload.as_bytes(), &Signature::from_bytes(&signature))
        .map_err(|_| CryptoError::Signature)
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
