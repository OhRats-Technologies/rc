use aes_gcm::{
    Aes256Gcm, KeyInit,
    aead::{Aead, Payload},
};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use ed25519_dalek::{Signature, Signer, SigningKey, Verifier, VerifyingKey};
use hkdf::Hkdf;
use sha2::{Digest, Sha256};
use x25519_dalek::{PublicKey, StaticSecret};

#[derive(Debug, thiserror::Error)]
pub enum CryptoError {
    #[error("invalid base64url")]
    Base64(#[from] base64::DecodeError),
    #[error("invalid key length")]
    KeyLength,
    #[error("key derivation failed")]
    Kdf,
    #[error("control frame authentication failed")]
    Authentication,
    #[error("invalid signature")]
    Signature,
}

pub fn session_payload(
    challenge: &str,
    device_id: &str,
    client_id: &str,
    public_key: &str,
) -> String {
    format!("rc-session-v1\n{challenge}\n{device_id}\n{client_id}\n{public_key}")
}

#[allow(clippy::too_many_arguments)]
pub fn ready_payload(
    challenge: &str,
    device_id: &str,
    client_id: &str,
    public_key: &str,
    transport_key: &str,
    ephemeral_key: &str,
    session_id: &str,
    attempt_plan: &str,
) -> String {
    if attempt_plan.is_empty() {
        return format!(
            "rc-ready-v2\n{challenge}\n{device_id}\n{client_id}\n{public_key}\n{transport_key}\n{ephemeral_key}\n{session_id}"
        );
    }
    format!(
        "rc-ready-v3\n{challenge}\n{device_id}\n{client_id}\n{public_key}\n{transport_key}\n{ephemeral_key}\n{session_id}\n{attempt_plan}"
    )
}

pub fn frame_nonce(direction: u8, sequence: u64) -> [u8; 12] {
    let mut nonce = [0_u8; 12];
    nonce[0] = direction;
    nonce[4..].copy_from_slice(&sequence.to_be_bytes());
    nonce
}

pub fn frame_aad(session_id: &str, sequence: u64, direction: &str) -> Vec<u8> {
    format!("rc-frame-v1\n{session_id}\n{sequence}\n{direction}").into_bytes()
}

pub fn decode_x25519(value: &str) -> Result<[u8; 32], CryptoError> {
    let bytes = URL_SAFE_NO_PAD.decode(value)?;
    bytes.try_into().map_err(|_| CryptoError::KeyLength)
}

pub fn x25519_public(private: &str) -> Result<String, CryptoError> {
    let secret = StaticSecret::from(decode_x25519(private)?);
    Ok(URL_SAFE_NO_PAD.encode(PublicKey::from(&secret).as_bytes()))
}

pub fn x25519_shared(private: &str, public: &str) -> Result<[u8; 32], CryptoError> {
    let secret = StaticSecret::from(decode_x25519(private)?);
    let peer = PublicKey::from(decode_x25519(public)?);
    Ok(*secret.diffie_hellman(&peer).as_bytes())
}

pub fn derive_session_key(
    shared_static: &[u8; 32],
    shared_ephemeral: &[u8; 32],
    challenge: &str,
    device_id: &str,
    client_id: &str,
) -> Result<[u8; 32], CryptoError> {
    let mut material = [0_u8; 64];
    material[..32].copy_from_slice(shared_static);
    material[32..].copy_from_slice(shared_ephemeral);
    let salt = Sha256::digest(challenge.as_bytes());
    let hkdf = Hkdf::<Sha256>::new(Some(&salt), &material);
    let mut key = [0_u8; 32];
    hkdf.expand(
        format!("rc-e2e-v2\n{device_id}\n{client_id}").as_bytes(),
        &mut key,
    )
    .map_err(|_| CryptoError::Kdf)?;
    Ok(key)
}

pub fn derive_client_key(
    client_private: &str,
    node_static_public: &str,
    node_ephemeral_public: &str,
    challenge: &str,
    device_id: &str,
    client_id: &str,
) -> Result<[u8; 32], CryptoError> {
    derive_session_key(
        &x25519_shared(client_private, node_static_public)?,
        &x25519_shared(client_private, node_ephemeral_public)?,
        challenge,
        device_id,
        client_id,
    )
}

pub fn derive_node_key(
    static_private: &str,
    ephemeral_private: &str,
    client_public: &str,
    challenge: &str,
    device_id: &str,
    client_id: &str,
) -> Result<[u8; 32], CryptoError> {
    derive_session_key(
        &x25519_shared(static_private, client_public)?,
        &x25519_shared(ephemeral_private, client_public)?,
        challenge,
        device_id,
        client_id,
    )
}

pub fn encrypt_frame(
    key: &[u8; 32],
    direction: u8,
    sequence: u64,
    session_id: &str,
    label: &str,
    plaintext: &[u8],
) -> Result<String, CryptoError> {
    let cipher = Aes256Gcm::new_from_slice(key).map_err(|_| CryptoError::KeyLength)?;
    let nonce = frame_nonce(direction, sequence);
    let aad = frame_aad(session_id, sequence, label);
    let value = cipher
        .encrypt(
            (&nonce).into(),
            Payload {
                msg: plaintext,
                aad: &aad,
            },
        )
        .map_err(|_| CryptoError::Authentication)?;
    Ok(URL_SAFE_NO_PAD.encode(value))
}

pub fn decrypt_frame(
    key: &[u8; 32],
    direction: u8,
    sequence: u64,
    session_id: &str,
    label: &str,
    ciphertext: &str,
) -> Result<Vec<u8>, CryptoError> {
    let cipher = Aes256Gcm::new_from_slice(key).map_err(|_| CryptoError::KeyLength)?;
    let nonce = frame_nonce(direction, sequence);
    let aad = frame_aad(session_id, sequence, label);
    let value = URL_SAFE_NO_PAD.decode(ciphertext)?;
    cipher
        .decrypt(
            (&nonce).into(),
            Payload {
                msg: &value,
                aad: &aad,
            },
        )
        .map_err(|_| CryptoError::Authentication)
}

pub fn sign_ed25519_seed(seed: &str, payload: &[u8]) -> Result<String, CryptoError> {
    let bytes: [u8; 32] = URL_SAFE_NO_PAD
        .decode(seed)?
        .try_into()
        .map_err(|_| CryptoError::KeyLength)?;
    Ok(URL_SAFE_NO_PAD.encode(SigningKey::from_bytes(&bytes).sign(payload).to_bytes()))
}

pub fn verify_ed25519(public: &str, payload: &[u8], signature: &str) -> Result<(), CryptoError> {
    let public: [u8; 32] = URL_SAFE_NO_PAD
        .decode(public)?
        .try_into()
        .map_err(|_| CryptoError::KeyLength)?;
    let signature: [u8; 64] = URL_SAFE_NO_PAD
        .decode(signature)?
        .try_into()
        .map_err(|_| CryptoError::KeyLength)?;
    VerifyingKey::from_bytes(&public)
        .map_err(|_| CryptoError::Signature)?
        .verify(payload, &Signature::from_bytes(&signature))
        .map_err(|_| CryptoError::Signature)
}
