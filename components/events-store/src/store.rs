use crate::{
    model::{StoredDetail, StoredEvent, StoredIdempotency, encode_kind},
    ohrats::rc_events::types::{AppendRequest, Detail, Event},
    storage, validate,
};
use sha2::{Digest, Sha256};
use std::collections::HashSet;

const NEXT: &[u8] = b"next-cursor";
const POLICY: &[u8] = b"maximum-events";
pub const DEFAULT_MAX: u32 = 10_000;
pub const MAX_RETENTION: u32 = 100_000;
const PRUNE_BATCH: usize = 500;

pub fn append(request: AppendRequest) -> Result<Event, String> {
    validate::request(&request)?;
    let idem = request.idempotency_key.as_deref().map(idempotency_key);
    let fingerprint = request_fingerprint(&request)?;
    if let Some(key) = idem.as_deref()
        && let Some(value) = storage::get(storage::IDEMPOTENCY, key)?
    {
        return replay(value, &fingerprint);
    }
    let mut result = None;
    storage::transact(|_| {
        if let Some(key) = idem.as_deref()
            && storage::get(storage::IDEMPOTENCY, key)?.is_some()
        {
            return Err("idempotency retry raced; retry request".into());
        }
        let cursor = storage::read_u64(storage::get(storage::META, NEXT)?)?
            .checked_add(1)
            .ok_or("event cursor exhausted")?;
        let stored = StoredEvent {
            cursor,
            kind: encode_kind(request.kind),
            occurred_at_ms: request.occurred_at_ms,
            actor_account_id: request.actor_account_id.clone(),
            detail: store_detail(&request.detail),
        };
        let encoded = serde_json::to_vec(&stored).map_err(display)?;
        let maximum = policy()? as usize;
        let entries = storage::scan(storage::EVENTS)?;
        let prune = entries.len().saturating_add(1).saturating_sub(maximum);
        let pruned_cursors = entries
            .iter()
            .take(prune)
            .map(|entry| entry.key.clone())
            .collect::<Vec<_>>();
        let mut changes = entries
            .into_iter()
            .take(prune)
            .map(|e| storage::delete(storage::EVENTS, e.key))
            .collect::<Vec<_>>();
        changes.extend(prune_idempotency(&pruned_cursors)?);
        changes.push(storage::put(
            storage::EVENTS,
            storage::cursor_key(cursor),
            encoded,
        ));
        changes.push(storage::put(
            storage::META,
            NEXT.to_vec(),
            cursor.to_be_bytes().to_vec(),
        ));
        if let Some(key) = idem.clone() {
            let value = serde_json::to_vec(&StoredIdempotency {
                cursor,
                request_sha256: fingerprint.clone(),
            })
            .map_err(display)?;
            changes.push(storage::put(storage::IDEMPOTENCY, key, value));
        }
        result = Some(stored.wire()?);
        Ok(changes)
    })?;
    result.ok_or_else(|| "event append produced no event".into())
}

fn replay(value: Vec<u8>, fingerprint: &[u8]) -> Result<Event, String> {
    let stored: StoredIdempotency = serde_json::from_slice(&value).map_err(display)?;
    let event = load(stored.cursor)?;
    if stored.request_sha256 != fingerprint {
        return Err("idempotency key was already used for a different event".into());
    }
    Ok(event)
}

fn request_fingerprint(request: &AppendRequest) -> Result<Vec<u8>, String> {
    let event = StoredEvent {
        cursor: 0,
        kind: encode_kind(request.kind),
        occurred_at_ms: request.occurred_at_ms,
        actor_account_id: request.actor_account_id.clone(),
        detail: store_detail(&request.detail),
    };
    let encoded = serde_json::to_vec(&event).map_err(display)?;
    Ok(Sha256::digest(encoded).to_vec())
}

pub fn load(cursor: u64) -> Result<Event, String> {
    let value = storage::get(storage::EVENTS, &storage::cursor_key(cursor))?
        .ok_or("idempotent event expired from retention")?;
    serde_json::from_slice::<StoredEvent>(&value)
        .map_err(display)?
        .wire()
}

pub fn policy() -> Result<u32, String> {
    let value = storage::read_u64(storage::get(storage::META, POLICY)?)?;
    if value == 0 {
        Ok(DEFAULT_MAX)
    } else {
        u32::try_from(value).map_err(display)
    }
}

pub fn configure(maximum: u32) -> Result<(), String> {
    if maximum == 0 || maximum > MAX_RETENTION {
        return Err("retention maximum is outside supported bounds".into());
    }
    loop {
        let mut complete = false;
        storage::transact(|_| {
            let entries = storage::scan(storage::EVENTS)?;
            let excess = entries.len().saturating_sub(maximum as usize);
            let prune = excess.min(PRUNE_BATCH);
            let pruned_cursors = entries
                .iter()
                .take(prune)
                .map(|entry| entry.key.clone())
                .collect::<Vec<_>>();
            let mut changes = entries
                .into_iter()
                .take(prune)
                .map(|e| storage::delete(storage::EVENTS, e.key))
                .collect::<Vec<_>>();
            changes.extend(prune_idempotency(&pruned_cursors)?);
            complete = excess == 0;
            if complete {
                changes.push(storage::put(
                    storage::META,
                    POLICY.to_vec(),
                    u64::from(maximum).to_be_bytes().to_vec(),
                ));
            }
            Ok(changes)
        })?;
        if complete {
            return Ok(());
        }
    }
}

fn prune_idempotency(
    pruned_cursors: &[Vec<u8>],
) -> Result<Vec<crate::ohrats::rc_storage::types::Change>, String> {
    if pruned_cursors.is_empty() {
        return Ok(Vec::new());
    }
    let cursors = pruned_cursors
        .iter()
        .map(|key| {
            let bytes: [u8; 8] = key
                .as_slice()
                .try_into()
                .map_err(|_| "invalid stored event cursor key")?;
            Ok(u64::from_be_bytes(bytes))
        })
        .collect::<Result<HashSet<_>, String>>()?;
    storage::scan(storage::IDEMPOTENCY)?
        .into_iter()
        .filter_map(
            |entry| match serde_json::from_slice::<StoredIdempotency>(&entry.value) {
                Ok(stored) if cursors.contains(&stored.cursor) => {
                    Some(Ok(storage::delete(storage::IDEMPOTENCY, entry.key)))
                }
                Ok(_) => None,
                Err(error) => Some(Err(format!(
                    "invalid stored event idempotency metadata: {error}"
                ))),
            },
        )
        .collect()
}

fn idempotency_key(value: &str) -> Vec<u8> {
    Sha256::digest(value.as_bytes()).to_vec()
}
fn store_detail(value: &Detail) -> StoredDetail {
    match value {
        Detail::Account(v) => StoredDetail::Account(v.account_id.clone(), v.display_name.clone()),
        Detail::Workspace(v) => StoredDetail::Workspace(v.workspace_id.clone(), v.name.clone()),
        Detail::Membership(v) => {
            StoredDetail::Membership(v.workspace_id.clone(), v.account_id.clone(), v.role.clone())
        }
        Detail::Invitation(v) => {
            StoredDetail::Invitation(v.workspace_id.clone(), v.invitation_id.clone())
        }
        Detail::Device(v) => {
            StoredDetail::Device(v.workspace_id.clone(), v.device_id.clone(), v.name.clone())
        }
    }
}
fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}
