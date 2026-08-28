use crate::{
    CapabilityAdvertisement, CapabilityError, MAX_CAPABILITIES, PeerId, RealmId, sign_payload,
    verify_payload,
};
use std::collections::BTreeSet;

pub const ADVERTISEMENT_VERSION: u32 = 1;

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Neighbor {
    pub peer_id: PeerId,
    pub cost: u32,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ServiceAdvertisement {
    pub name: String,
    pub cost: u32,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct LinkAdvertisement {
    pub v: u32,
    pub realm_id: RealmId,
    pub origin: PeerId,
    pub sequence: u64,
    pub issued_at: i64,
    pub expires_at: i64,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub capabilities: Vec<CapabilityAdvertisement>,
    #[serde(default)]
    pub neighbors: Vec<Neighbor>,
    #[serde(default)]
    pub services: Vec<ServiceAdvertisement>,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SignedAdvertisement {
    pub advertisement: LinkAdvertisement,
    pub signature: String,
}

impl SignedAdvertisement {
    pub fn sign(advertisement: LinkAdvertisement, seed: &str) -> anyhow::Result<Self> {
        validate_shape(&advertisement)?;
        let payload = serde_json::to_vec(&advertisement)?;
        let signature = sign_payload(seed, "rc-mesh-link-v1", &payload)?;
        Ok(Self {
            advertisement,
            signature,
        })
    }

    pub fn verify(&self, public_key: &str, now_ms: i64) -> Result<(), AdvertisementError> {
        validate_shape(&self.advertisement)?;
        let expected =
            PeerId::from_public_key(public_key).map_err(|_| AdvertisementError::Identity)?;
        if expected != self.advertisement.origin {
            return Err(AdvertisementError::Identity);
        }
        if self.advertisement.issued_at > now_ms + 60_000
            || self.advertisement.expires_at <= now_ms
            || self.advertisement.expires_at <= self.advertisement.issued_at
        {
            return Err(AdvertisementError::Expired);
        }
        let payload =
            serde_json::to_vec(&self.advertisement).map_err(|_| AdvertisementError::Encoding)?;
        verify_payload(public_key, "rc-mesh-link-v1", &payload, &self.signature)
            .map_err(|_| AdvertisementError::Signature)
    }
}

fn validate_shape(advertisement: &LinkAdvertisement) -> Result<(), AdvertisementError> {
    let mut capabilities = BTreeSet::new();
    let mut neighbors = BTreeSet::new();
    let mut services = BTreeSet::new();
    if advertisement.v != ADVERTISEMENT_VERSION
        || advertisement.realm_id.as_str().is_empty()
        || advertisement.origin.as_str().is_empty()
        || advertisement.capabilities.len() > MAX_CAPABILITIES
        || advertisement.neighbors.len() > 256
        || advertisement.services.len() > 64
        || advertisement.neighbors.iter().any(|neighbor| {
            neighbor.cost == 0
                || neighbor.peer_id == advertisement.origin
                || !neighbors.insert(neighbor.peer_id.clone())
        })
        || advertisement.services.iter().any(|service| {
            service.name.is_empty()
                || service.name.len() > 128
                || service.cost == 0
                || !services.insert(service.name.clone())
        })
    {
        return Err(AdvertisementError::Shape);
    }
    for capability in &advertisement.capabilities {
        capability.validate()?;
        if !capabilities.insert(capability.id.clone()) {
            return Err(AdvertisementError::Shape);
        }
    }
    Ok(())
}

#[derive(Debug, thiserror::Error)]
pub enum AdvertisementError {
    #[error("invalid mesh advertisement")]
    Shape,
    #[error("mesh advertisement identity mismatch")]
    Identity,
    #[error("mesh advertisement expired")]
    Expired,
    #[error("invalid mesh advertisement signature")]
    Signature,
    #[error("mesh advertisement encoding failed")]
    Encoding,
    #[error(transparent)]
    Capability(#[from] CapabilityError),
}
