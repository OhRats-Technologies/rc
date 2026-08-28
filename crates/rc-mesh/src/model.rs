use bytes::Bytes;
use serde::{Deserialize, Serialize};
use std::fmt;

pub const MAX_ENVELOPE_PAYLOAD: usize = 1_048_576;

#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(transparent)]
pub struct RealmId(String);

#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(transparent)]
pub struct PeerId(String);

#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(transparent)]
pub struct ServiceId(String);

#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", tag = "kind", content = "id")]
pub enum RouteTarget {
    Device(PeerId),
    Service(ServiceId),
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RouteDescriptor {
    pub realm: RealmId,
    pub target: RouteTarget,
    pub via: PeerId,
    pub cost: u32,
    pub expires_at: i64,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct MeshEnvelope {
    pub version: u8,
    pub realm: RealmId,
    pub message_id: String,
    pub source: PeerId,
    pub destination: PeerId,
    pub expires_at: i64,
    pub hop_limit: u8,
    pub payload: Bytes,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MeshPolicy {
    pub enabled: bool,
    pub authenticated_relay: bool,
    pub lan_discovery: bool,
    pub public_relay: bool,
    pub maximum_hops: u8,
}

impl Default for MeshPolicy {
    fn default() -> Self {
        Self {
            enabled: true,
            authenticated_relay: true,
            lan_discovery: true,
            public_relay: false,
            maximum_hops: 8,
        }
    }
}

impl RealmId {
    pub fn new(value: impl Into<String>) -> Result<Self, IdentifierError> {
        validate_identifier(value.into()).map(Self)
    }

    pub fn as_str(&self) -> &str {
        &self.0
    }
}

impl PeerId {
    pub fn new(value: impl Into<String>) -> Result<Self, IdentifierError> {
        validate_identifier(value.into()).map(Self)
    }

    pub fn as_str(&self) -> &str {
        &self.0
    }
}

impl ServiceId {
    pub fn new(value: impl Into<String>) -> Result<Self, IdentifierError> {
        validate_identifier(value.into()).map(Self)
    }

    pub fn as_str(&self) -> &str {
        &self.0
    }
}

impl MeshEnvelope {
    pub fn validate(&self, now_ms: i64, maximum_hops: u8) -> Result<(), EnvelopeError> {
        if self.version != 1 || self.message_id.is_empty() || self.message_id.len() > 128 {
            return Err(EnvelopeError::Invalid);
        }
        if self.source == self.destination
            || self.payload.is_empty()
            || self.payload.len() > MAX_ENVELOPE_PAYLOAD
        {
            return Err(EnvelopeError::Invalid);
        }
        if self.expires_at <= now_ms {
            return Err(EnvelopeError::Expired);
        }
        if self.hop_limit == 0 || self.hop_limit > maximum_hops {
            return Err(EnvelopeError::HopLimit);
        }
        Ok(())
    }

    pub fn forwarded(&self) -> Result<Self, EnvelopeError> {
        let hop_limit = self
            .hop_limit
            .checked_sub(1)
            .filter(|value| *value > 0)
            .ok_or(EnvelopeError::HopLimit)?;
        let mut next = self.clone();
        next.hop_limit = hop_limit;
        Ok(next)
    }
}

impl fmt::Display for RealmId {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(&self.0)
    }
}

impl fmt::Display for PeerId {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(&self.0)
    }
}

impl fmt::Display for ServiceId {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(&self.0)
    }
}

#[derive(Debug, thiserror::Error)]
#[error("mesh identifiers must be 1-128 visible ASCII characters")]
pub struct IdentifierError;

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum EnvelopeError {
    #[error("invalid mesh envelope")]
    Invalid,
    #[error("mesh envelope expired")]
    Expired,
    #[error("mesh envelope hop limit exhausted")]
    HopLimit,
}

fn validate_identifier(value: String) -> Result<String, IdentifierError> {
    if value.is_empty()
        || value.len() > 128
        || !value
            .bytes()
            .all(|byte| byte.is_ascii_graphic() && !byte.is_ascii_whitespace())
    {
        return Err(IdentifierError);
    }
    Ok(value)
}
