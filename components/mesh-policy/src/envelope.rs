use crate::{
    ohrats::rc_mesh::types::{Envelope, Rejection},
    validate::{self, MAX_CIPHERTEXT_BYTES},
};

pub fn validate(value: &Envelope, now_ms: u64, maximum_hops: u8) -> Result<(), Rejection> {
    validate::identifier(&value.realm_id)?;
    validate::identifier(&value.source_peer_id)?;
    validate::identifier(&value.destination_peer_id)?;
    if value.version != 1
        || value.message_id.is_empty()
        || value.message_id.len() > 128
        || value.source_peer_id == value.destination_peer_id
        || value.ciphertext.is_empty()
        || value.ciphertext.len() > MAX_CIPHERTEXT_BYTES
        || value.issued_at_ms > now_ms.saturating_add(60_000)
        || value.expires_at_ms <= now_ms
        || value.expires_at_ms <= value.issued_at_ms
        || value.max_hops == 0
        || value.max_hops > maximum_hops
        || value.max_hops > 32
        || value.route.len() > usize::from(value.max_hops)
        || value
            .route
            .iter()
            .any(|peer| validate::identifier(peer).is_err())
    {
        return Err(if value.expires_at_ms <= now_ms {
            Rejection::Expired
        } else {
            Rejection::InvalidEnvelope
        });
    }
    Ok(())
}

pub fn forward(mut value: Envelope, relay_peer_id: &str) -> Result<Envelope, Rejection> {
    validate::identifier(relay_peer_id)?;
    if relay_peer_id == value.destination_peer_id
        || value.route.iter().any(|peer| peer == relay_peer_id)
        || value.route.len() >= usize::from(value.max_hops)
    {
        return Err(Rejection::RouteRejected);
    }
    value.route.push(relay_peer_id.into());
    Ok(value)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn envelope() -> Envelope {
        Envelope {
            version: 1,
            realm_id: "realm-a".into(),
            message_id: "message-a".into(),
            source_peer_id: "peer-a".into(),
            destination_peer_id: "peer-c".into(),
            issued_at_ms: 1_000,
            expires_at_ms: 5_000,
            max_hops: 4,
            ciphertext: b"ciphertext".to_vec(),
            route: Vec::new(),
        }
    }

    #[test]
    fn rejects_relay_loops() {
        let first = forward(envelope(), "peer-b").unwrap();
        assert!(forward(first, "peer-b").is_err());
    }
}
