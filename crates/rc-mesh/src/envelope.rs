use crate::{PeerId, RealmId, sign_payload, verify_payload};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use parking_lot::Mutex;
use std::collections::HashMap;

pub const ENVELOPE_VERSION: u32 = 1;

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct EnvelopeHeader {
    pub v: u32,
    pub realm_id: RealmId,
    pub message_id: String,
    pub source: PeerId,
    pub destination: PeerId,
    pub issued_at: i64,
    pub expires_at: i64,
    pub max_hops: u8,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SignedMeshEnvelope {
    pub header: EnvelopeHeader,
    pub ciphertext: String,
    pub signature: String,
    #[serde(default)]
    pub route: Vec<PeerId>,
}

impl SignedMeshEnvelope {
    pub fn sign(header: EnvelopeHeader, ciphertext: &[u8], seed: &str) -> anyhow::Result<Self> {
        validate_header(&header)?;
        if ciphertext.is_empty() || ciphertext.len() > 1_500_000 {
            return Err(SignedEnvelopeError::Shape.into());
        }
        let encoded = URL_SAFE_NO_PAD.encode(ciphertext);
        let payload = signed_payload(&header, &encoded)?;
        let signature = sign_payload(seed, "rc-mesh-envelope-v1", &payload)?;
        Ok(Self {
            header,
            ciphertext: encoded,
            signature,
            route: Vec::new(),
        })
    }

    pub fn verify(
        &self,
        source_public_key: &str,
        now_ms: i64,
    ) -> Result<Vec<u8>, SignedEnvelopeError> {
        validate_header(&self.header)?;
        if self.header.issued_at > now_ms + 60_000 || self.header.expires_at <= now_ms {
            return Err(SignedEnvelopeError::Expired);
        }
        let expected = PeerId::from_public_key(source_public_key)
            .map_err(|_| SignedEnvelopeError::Identity)?;
        if expected != self.header.source {
            return Err(SignedEnvelopeError::Identity);
        }
        let ciphertext = URL_SAFE_NO_PAD
            .decode(&self.ciphertext)
            .map_err(|_| SignedEnvelopeError::Encoding)?;
        if ciphertext.is_empty() || ciphertext.len() > 1_500_000 {
            return Err(SignedEnvelopeError::Shape);
        }
        let payload = signed_payload(&self.header, &self.ciphertext)
            .map_err(|_| SignedEnvelopeError::Encoding)?;
        verify_payload(
            source_public_key,
            "rc-mesh-envelope-v1",
            &payload,
            &self.signature,
        )
        .map_err(|_| SignedEnvelopeError::Signature)?;
        Ok(ciphertext)
    }

    pub fn forward(&self, relay: &PeerId) -> Result<Self, SignedEnvelopeError> {
        if self.header.destination == *relay
            || self.route.iter().any(|peer| peer == relay)
            || self.route.len() >= usize::from(self.header.max_hops)
        {
            return Err(SignedEnvelopeError::Route);
        }
        let mut next = self.clone();
        next.route.push(relay.clone());
        Ok(next)
    }
}

fn validate_header(header: &EnvelopeHeader) -> Result<(), SignedEnvelopeError> {
    if header.v != ENVELOPE_VERSION
        || header.realm_id.as_str().is_empty()
        || header.message_id.is_empty()
        || header.message_id.len() > 128
        || header.source.as_str().is_empty()
        || header.destination.as_str().is_empty()
        || header.source == header.destination
        || header.max_hops == 0
        || header.max_hops > 32
        || header.expires_at <= header.issued_at
    {
        return Err(SignedEnvelopeError::Shape);
    }
    Ok(())
}

fn signed_payload(header: &EnvelopeHeader, ciphertext: &str) -> anyhow::Result<Vec<u8>> {
    Ok(serde_json::to_vec(&(header, ciphertext))?)
}

#[derive(Default)]
pub struct ReplayGuard {
    seen: Mutex<HashMap<(PeerId, String), i64>>,
}

impl ReplayGuard {
    pub fn accept(&self, envelope: &SignedMeshEnvelope, now_ms: i64) -> bool {
        let mut seen = self.seen.lock();
        seen.retain(|_, expires_at| *expires_at > now_ms);
        let key = (
            envelope.header.source.clone(),
            envelope.header.message_id.clone(),
        );
        if seen.contains_key(&key) {
            return false;
        }
        seen.insert(key, envelope.header.expires_at);
        true
    }
}

#[derive(Debug, thiserror::Error)]
pub enum SignedEnvelopeError {
    #[error("invalid mesh envelope")]
    Shape,
    #[error("mesh envelope expired")]
    Expired,
    #[error("mesh envelope identity mismatch")]
    Identity,
    #[error("invalid mesh envelope signature")]
    Signature,
    #[error("invalid mesh envelope encoding")]
    Encoding,
    #[error("mesh envelope route rejected")]
    Route,
}
