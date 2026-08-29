use crate::ohrats::rc_mesh::types::{CapabilityAdvertisement, LinkAdvertisement, Rejection};
use std::collections::BTreeSet;

pub const MAX_CAPABILITIES: usize = 64;
pub const MAX_CAPABILITY_VERSIONS: usize = 16;
pub const MAX_CAPABILITY_FEATURES: usize = 32;
pub const MAX_NEIGHBORS: usize = 256;
pub const MAX_SERVICES: usize = 64;
pub const MAX_CIPHERTEXT_BYTES: usize = 1_500_000;

pub fn identifier(value: &str) -> Result<(), Rejection> {
    if value.is_empty()
        || value.len() > 128
        || !value
            .bytes()
            .all(|byte| byte.is_ascii_graphic() && !byte.is_ascii_whitespace())
    {
        Err(Rejection::InvalidIdentifier)
    } else {
        Ok(())
    }
}

pub fn capability(value: &CapabilityAdvertisement) -> Result<(), Rejection> {
    if !token(&value.id, 128, true)
        || value.versions.is_empty()
        || value.versions.len() > MAX_CAPABILITY_VERSIONS
        || value.versions.contains(&0)
        || !sorted(&value.versions)
        || value.features.len() > MAX_CAPABILITY_FEATURES
        || !sorted(&value.features)
        || value
            .features
            .iter()
            .any(|feature| !token(feature, 64, false))
    {
        Err(Rejection::InvalidCapability)
    } else {
        Ok(())
    }
}

pub fn advertisement(value: &LinkAdvertisement) -> Result<(), Rejection> {
    identifier(&value.realm_id)?;
    identifier(&value.origin_peer_id)?;
    let mut capabilities = BTreeSet::new();
    let mut neighbors = BTreeSet::new();
    let mut services = BTreeSet::new();
    if value.version != 1
        || value.expires_at_ms <= value.issued_at_ms
        || value.capabilities.len() > MAX_CAPABILITIES
        || value.neighbors.len() > MAX_NEIGHBORS
        || value.services.len() > MAX_SERVICES
        || value.neighbors.iter().any(|neighbor| {
            identifier(&neighbor.peer_id).is_err()
                || neighbor.cost == 0
                || neighbor.peer_id == value.origin_peer_id
                || !neighbors.insert(neighbor.peer_id.clone())
        })
        || value.services.iter().any(|service| {
            service.name.is_empty()
                || service.name.len() > 128
                || service.cost == 0
                || !services.insert(service.name.clone())
        })
    {
        return Err(Rejection::InvalidAdvertisement);
    }
    for item in &value.capabilities {
        capability(item)?;
        if !capabilities.insert(item.id.clone()) {
            return Err(Rejection::InvalidAdvertisement);
        }
    }
    Ok(())
}

fn sorted<T: Ord>(values: &[T]) -> bool {
    values.windows(2).all(|pair| pair[0] < pair[1])
}

fn token(value: &str, maximum: usize, slash: bool) -> bool {
    !value.is_empty()
        && value.len() <= maximum
        && value.bytes().all(|byte| {
            byte.is_ascii_lowercase()
                || byte.is_ascii_digit()
                || matches!(byte, b'.' | b'-' | b'_')
                || (slash && byte == b'/')
        })
        && value
            .as_bytes()
            .first()
            .is_some_and(u8::is_ascii_alphanumeric)
        && value
            .as_bytes()
            .last()
            .is_some_and(u8::is_ascii_alphanumeric)
}

#[cfg(test)]
mod tests {
    use super::identifier;

    #[test]
    fn identifiers_are_bounded_visible_ascii() {
        assert!(identifier("peer-a").is_ok());
        assert!(identifier("").is_err());
        assert!(identifier("peer a").is_err());
    }
}
