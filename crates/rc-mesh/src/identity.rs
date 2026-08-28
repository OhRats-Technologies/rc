use crate::PeerId;
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use sha2::{Digest, Sha256};

impl PeerId {
    pub fn from_public_key(public_key: &str) -> Result<Self, IdentityError> {
        let bytes = URL_SAFE_NO_PAD
            .decode(public_key)
            .map_err(|_| IdentityError::PublicKey)?;
        if bytes.len() != 32 {
            return Err(IdentityError::PublicKey);
        }
        let digest = Sha256::digest(bytes);
        Self::new(URL_SAFE_NO_PAD.encode(&digest[..20])).map_err(|_| IdentityError::PublicKey)
    }
}

#[derive(Debug, thiserror::Error)]
pub enum IdentityError {
    #[error("invalid mesh identity public key")]
    PublicKey,
}

pub fn sign_payload(
    seed: &str,
    domain: &str,
    payload: &[u8],
) -> Result<String, rc_crypto::CryptoError> {
    let mut message = Vec::with_capacity(domain.len() + payload.len() + 1);
    message.extend_from_slice(domain.as_bytes());
    message.push(b'\n');
    message.extend_from_slice(payload);
    rc_crypto::sign_ed25519_seed(seed, &message)
}

pub fn verify_payload(
    public_key: &str,
    domain: &str,
    payload: &[u8],
    signature: &str,
) -> Result<(), rc_crypto::CryptoError> {
    let mut message = Vec::with_capacity(domain.len() + payload.len() + 1);
    message.extend_from_slice(domain.as_bytes());
    message.push(b'\n');
    message.extend_from_slice(payload);
    rc_crypto::verify_ed25519(public_key, &message, signature)
}
