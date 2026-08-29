use crate::{
    model::{self, DEVICES, IDENTITIES, StoredDevice, StoredToken, TOKENS},
    ohrats::{
        rc_devices::types::{Device, EnrollmentError, EnrollmentInput, IssuedEnrollment},
        rc_storage::durable_store,
    },
    registry, storage, validate,
};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use sha2::{Digest, Sha256};

const DEVICE_LIMIT: usize = 25;
const ACTIVE_TOKEN_LIMIT: usize = 25;
const USED_TOKEN_LIMIT: usize = 25;
const TOKEN_RETENTION_MS: u64 = 30 * 24 * 60 * 60 * 1000;
const MAX_TOKEN_LIFETIME_MS: u64 = 24 * 60 * 60 * 1000;
const MAX_TOKEN_DELETIONS: usize = 256;

pub fn issue(
    workspace: String,
    created_by: String,
    now_ms: u64,
    expires_at_ms: u64,
) -> Result<IssuedEnrollment, String> {
    validate::id(&workspace, "workspace id")?;
    validate::id(&created_by, "creator id")?;
    if now_ms == 0 || expires_at_ms <= now_ms || expires_at_ms - now_ms > MAX_TOKEN_LIFETIME_MS {
        return Err("invalid enrollment lifetime".into());
    }
    let mut random = [0_u8; 32];
    getrandom::fill(&mut random).map_err(|error| error.to_string())?;
    let token = format!("enroll_{}", URL_SAFE_NO_PAD.encode(random));
    let record = StoredToken {
        workspace_id: workspace.clone(),
        created_by,
        created_at_ms: now_ms,
        expires_at_ms,
        device_id: None,
        identity_public_key: None,
        used_at_ms: None,
    };
    for _ in 0..storage::RETRIES {
        if registry::list(Some(&workspace))?.len() >= DEVICE_LIMIT {
            return Err("workspace device limit reached".into());
        }
        let (revision, tokens) = storage::scan(TOKENS)?;
        let active = tokens
            .iter()
            .filter_map(|entry| model::decode::<StoredToken>(&entry.value).ok())
            .filter(|value| {
                value.workspace_id == workspace
                    && value.used_at_ms.is_none()
                    && value.expires_at_ms > now_ms
            })
            .count();
        if active >= ACTIVE_TOKEN_LIMIT {
            return Err("workspace enrollment limit reached".into());
        }
        let mut changes = stale_token_changes(&tokens, &workspace, now_ms)?;
        changes.push(storage::put(TOKENS, hash(&token), model::encode(&record)?));
        if storage::commit(revision, &changes)? {
            return Ok(IssuedEnrollment {
                token,
                expires_at_ms,
            });
        }
    }
    Err("device state changed too frequently".into())
}

fn stale_token_changes(
    tokens: &[crate::ohrats::rc_storage::types::Entry],
    workspace: &str,
    now_ms: u64,
) -> Result<Vec<crate::ohrats::rc_storage::types::Change>, String> {
    let decoded = tokens
        .iter()
        .map(|entry| model::decode::<StoredToken>(&entry.value).map(|token| (entry, token)))
        .collect::<Result<Vec<_>, _>>()?;
    let mut used = decoded
        .iter()
        .filter(|(_, token)| token.workspace_id == workspace && token.used_at_ms.is_some())
        .collect::<Vec<_>>();
    used.sort_by_key(|(_, token)| token.used_at_ms);
    let excess = used.len().saturating_sub(USED_TOKEN_LIMIT);
    let mut keys = used
        .into_iter()
        .take(excess)
        .map(|(entry, _)| entry.key.clone())
        .collect::<Vec<_>>();
    for (entry, token) in decoded {
        if !keys.contains(&entry.key) && token_is_stale(&token, now_ms) {
            keys.push(entry.key.clone());
        }
    }
    keys.truncate(MAX_TOKEN_DELETIONS);
    Ok(keys
        .into_iter()
        .map(|key| storage::delete(TOKENS, key))
        .collect())
}

fn token_is_stale(token: &StoredToken, now_ms: u64) -> bool {
    match token.used_at_ms {
        Some(used_at_ms) => used_at_ms.saturating_add(TOKEN_RETENTION_MS) <= now_ms,
        None => token.expires_at_ms <= now_ms,
    }
}

pub fn consume(
    token: String,
    now_ms: u64,
    input: EnrollmentInput,
) -> Result<Device, EnrollmentError> {
    let prepared = prepare(input, now_ms).map_err(EnrollmentError::InvalidInput)?;
    let token_key = hash(&token);
    for _ in 0..storage::RETRIES {
        let revision = durable_store::revision().map_err(EnrollmentError::Failure)?;
        let Some(raw) = storage::get(TOKENS, &token_key).map_err(EnrollmentError::Failure)? else {
            return Err(EnrollmentError::InvalidToken);
        };
        let mut record: StoredToken = model::decode(&raw).map_err(EnrollmentError::Failure)?;
        if let Some(device_id) = &record.device_id {
            return retry(&record, device_id, &prepared.identity_public_key);
        }
        if record.expires_at_ms <= now_ms {
            return Err(EnrollmentError::ExpiredToken);
        }
        if let Some(existing) = registry::identity_device(&prepared.identity_public_key)
            .map_err(EnrollmentError::Failure)?
        {
            return Err(EnrollmentError::DuplicateIdentity(existing));
        }
        let (_, devices) = storage::scan(DEVICES).map_err(EnrollmentError::Failure)?;
        if devices
            .iter()
            .filter(|entry| {
                model::decode::<StoredDevice>(&entry.value)
                    .is_ok_and(|device| device.workspace_id == record.workspace_id)
            })
            .count()
            >= DEVICE_LIMIT
        {
            return Err(EnrollmentError::DeviceLimit);
        }
        let device = StoredDevice {
            id: random_id().map_err(EnrollmentError::Failure)?,
            workspace_id: record.workspace_id.clone(),
            name: prepared.name.clone(),
            hostname: prepared.hostname.clone(),
            platform: prepared.platform.clone(),
            arch: prepared.arch.clone(),
            identity_public_key: prepared.identity_public_key.clone(),
            transport_public_key: prepared.transport_public_key.clone(),
            version: prepared.version.clone(),
            capabilities: prepared.capabilities.clone(),
            last_seen_at_ms: None,
            created_at_ms: now_ms,
        };
        record.device_id = Some(device.id.clone());
        record.identity_public_key = Some(device.identity_public_key.clone());
        record.used_at_ms = Some(now_ms);
        let changes = [
            registry::device_change(&device).map_err(EnrollmentError::Failure)?,
            storage::put(
                IDENTITIES,
                device.identity_public_key.as_bytes().to_vec(),
                device.id.as_bytes().to_vec(),
            ),
            storage::put(
                TOKENS,
                token_key.clone(),
                model::encode(&record).map_err(EnrollmentError::Failure)?,
            ),
        ];
        if storage::commit(revision, &changes).map_err(EnrollmentError::Failure)? {
            return Ok(device.into());
        }
    }
    Err(EnrollmentError::Failure(
        "device state changed too frequently".into(),
    ))
}

fn retry(record: &StoredToken, device_id: &str, identity: &str) -> Result<Device, EnrollmentError> {
    if record.identity_public_key.as_deref() != Some(identity) {
        return Err(EnrollmentError::TokenUsed);
    }
    registry::get(device_id)
        .map_err(EnrollmentError::Failure)?
        .ok_or_else(|| EnrollmentError::Failure("enrollment device missing".into()))
}

fn prepare(mut input: EnrollmentInput, now_ms: u64) -> Result<EnrollmentInput, String> {
    if now_ms == 0 {
        return Err("invalid enrollment time".into());
    }
    input.name = validate::text(&input.name, "device name", 120)?;
    input.hostname = validate::text(&input.hostname, "hostname", 255)?;
    input.platform = validate::text(&input.platform, "platform", 32)?;
    input.arch = validate::text(&input.arch, "architecture", 32)?;
    input.version = validate::text(&input.version, "version", 64)?;
    validate::key(&input.identity_public_key, "identity public key")?;
    validate::key(&input.transport_public_key, "transport public key")?;
    input.capabilities = validate::capabilities(&input.capabilities)?;
    Ok(input)
}

fn hash(token: &str) -> Vec<u8> {
    Sha256::digest(token.as_bytes()).to_vec()
}

fn random_id() -> Result<String, String> {
    let mut bytes = [0_u8; 16];
    getrandom::fill(&mut bytes).map_err(|error| error.to_string())?;
    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    let hex = bytes
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect::<String>();
    Ok(format!(
        "{}-{}-{}-{}-{}",
        &hex[..8],
        &hex[8..12],
        &hex[12..16],
        &hex[16..20],
        &hex[20..]
    ))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ohrats::rc_storage::types::Entry;

    #[test]
    fn cleanup_bounds_recent_used_tokens_and_removes_stale_tokens() {
        let now = TOKEN_RETENTION_MS + 10_000;
        let mut entries = (0..30)
            .map(|index| token_entry(index, "target", now - 1_000 + index, Some(now - 100)))
            .collect::<Vec<_>>();
        entries.push(token_entry(99, "other", 1, None));
        let changes = stale_token_changes(&entries, "target", now).unwrap();
        assert_eq!(changes.len(), 6);
    }

    fn token_entry(index: u64, workspace: &str, expires: u64, used: Option<u64>) -> Entry {
        Entry {
            bucket: TOKENS.into(),
            key: index.to_be_bytes().to_vec(),
            value: model::encode(&StoredToken {
                workspace_id: workspace.into(),
                created_by: "owner".into(),
                created_at_ms: 1,
                expires_at_ms: expires,
                device_id: used.map(|_| format!("device-{index}")),
                identity_public_key: used.map(|_| "identity".into()),
                used_at_ms: used,
            })
            .unwrap(),
        }
    }
}
