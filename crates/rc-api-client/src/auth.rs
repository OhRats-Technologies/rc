use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use ed25519_dalek::{Signer, SigningKey, pkcs8::DecodePrivateKey};
use rand::RngCore;
use rc_crypto::api_payload;

#[derive(Clone)]
pub enum Credential {
    Bearer(String),
    Pop(Box<PopKey>),
}

#[derive(Clone)]
pub struct PopKey {
    pub id: String,
    signing: SigningKey,
}

#[derive(Debug, thiserror::Error)]
pub enum AuthError {
    #[error(
        "RC credential required; sign in with rc login, or pass a PoP API key with --token / RC_API_TOKEN"
    )]
    Missing,
    #[error("invalid RC API signing key")]
    InvalidPop,
}

impl Credential {
    pub fn parse(value: &str) -> Result<Self, AuthError> {
        let value = value.trim();
        if value.is_empty() {
            return Err(AuthError::Missing);
        }
        if value.starts_with("rcsk_") {
            return Ok(Self::Pop(Box::new(PopKey::parse(value)?)));
        }
        Ok(Self::Bearer(value.to_owned()))
    }

    pub fn from_signing_seed(id: &str, seed: &str) -> Result<Self, AuthError> {
        let id = id.trim();
        if id.is_empty() {
            return Err(AuthError::InvalidPop);
        }
        let seed: [u8; 32] = URL_SAFE_NO_PAD
            .decode(seed.trim())
            .map_err(|_| AuthError::InvalidPop)?
            .try_into()
            .map_err(|_| AuthError::InvalidPop)?;
        Ok(Self::Pop(Box::new(PopKey {
            id: id.to_owned(),
            signing: SigningKey::from_bytes(&seed),
        })))
    }
}

impl PopKey {
    pub fn parse(secret: &str) -> Result<Self, AuthError> {
        let value = secret.strip_prefix("rcsk_").ok_or(AuthError::InvalidPop)?;
        let (id, encoded) = value.split_once('_').ok_or(AuthError::InvalidPop)?;
        if id.is_empty() {
            return Err(AuthError::InvalidPop);
        }
        let der = URL_SAFE_NO_PAD
            .decode(encoded)
            .map_err(|_| AuthError::InvalidPop)?;
        let signing = SigningKey::from_pkcs8_der(&der).map_err(|_| AuthError::InvalidPop)?;
        Ok(Self {
            id: id.to_owned(),
            signing,
        })
    }

    pub fn headers(&self, method: &str, request_uri: &str, body: &[u8]) -> [(String, String); 4] {
        let timestamp = unix_seconds().to_string();
        let nonce = random_url_bytes(18);
        let payload = api_payload(&self.id, &timestamp, &nonce, method, request_uri, body);
        let signature = URL_SAFE_NO_PAD.encode(self.signing.sign(payload.as_bytes()).to_bytes());
        [
            ("x-rc-key-id".into(), self.id.clone()),
            ("x-rc-timestamp".into(), timestamp),
            ("x-rc-nonce".into(), nonce),
            ("x-rc-signature".into(), signature),
        ]
    }

    pub fn signing_key(&self) -> &SigningKey {
        &self.signing
    }
}

pub fn random_url_bytes(size: usize) -> String {
    let mut bytes = vec![0_u8; size];
    rand::rngs::OsRng.fill_bytes(&mut bytes);
    URL_SAFE_NO_PAD.encode(bytes)
}

fn unix_seconds() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}
