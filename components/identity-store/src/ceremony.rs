use crate::{ohrats::rc_identity::types::Ceremony, storage, time, validate};
use serde::{Deserialize, Serialize};

const BUCKET: &str = "ceremonies";

#[derive(Serialize, Deserialize)]
struct StoredCeremony {
    id: String,
    kind: String,
    user_id: Option<String>,
    metadata: Vec<u8>,
    state: Vec<u8>,
    expires_at_ms: u64,
}

pub fn put(value: Ceremony) -> Result<(), String> {
    validate::id(&value.id, "ceremony id")?;
    validate::kind(&value.kind)?;
    if let Some(user_id) = value.user_id.as_deref() {
        validate::id(user_id, "user id")?;
    }
    validate::ceremony_payload(&value.metadata, &value.state)?;
    if value.expires_at_ms <= time::now_ms() {
        return Err("ceremony expiration must be in the future".into());
    }
    let key = key(&value.id, &value.kind);
    let stored = StoredCeremony {
        id: value.id,
        kind: value.kind,
        user_id: value.user_id,
        metadata: value.metadata,
        state: value.state,
        expires_at_ms: value.expires_at_ms,
    };
    let encoded = serde_json::to_vec(&stored).map_err(display)?;
    storage::insert(BUCKET, &key, encoded)
}

pub fn take(id: &str, kind: &str) -> Result<Option<Ceremony>, String> {
    validate::id(id, "ceremony id")?;
    validate::kind(kind)?;
    let Some(value) = storage::take(BUCKET, &key(id, kind))? else {
        return Ok(None);
    };
    let stored: StoredCeremony = serde_json::from_slice(&value).map_err(display)?;
    if stored.expires_at_ms <= time::now_ms() {
        return Ok(None);
    }
    Ok(Some(stored.into()))
}

fn key(id: &str, kind: &str) -> Vec<u8> {
    let mut value = Vec::with_capacity(kind.len() + id.len() + 1);
    value.extend_from_slice(kind.as_bytes());
    value.push(0);
    value.extend_from_slice(id.as_bytes());
    value
}

impl From<StoredCeremony> for Ceremony {
    fn from(value: StoredCeremony) -> Self {
        Self {
            id: value.id,
            kind: value.kind,
            user_id: value.user_id,
            metadata: value.metadata,
            state: value.state,
            expires_at_ms: value.expires_at_ms,
        }
    }
}

fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}
