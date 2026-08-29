use crate::{
    model::{self, PRESENCES, StoredPresence},
    ohrats::{
        rc_devices::types::{NodeStatus, NodeUpdate, Presence},
        rc_storage::durable_store,
    },
    registry, storage, validate,
};

pub fn renew(
    id: &str,
    identity: &str,
    now_ms: u64,
    lease_expires_at_ms: u64,
    update: NodeUpdate,
) -> Result<NodeStatus, String> {
    if now_ms == 0 || lease_expires_at_ms <= now_ms {
        return Err("invalid presence lease".into());
    }
    match registry::status(id, identity)? {
        NodeStatus::Active(_) => {}
        status => return Ok(status),
    }
    let hostname = validate::text(&update.hostname, "hostname", 255)?;
    let platform = validate::text(&update.platform, "platform", 32)?;
    let arch = validate::text(&update.arch, "architecture", 32)?;
    let version = validate::text(&update.version, "version", 64)?;
    let capabilities = validate::capabilities(&update.capabilities)?;
    let lock_hash = validate_lock_hash(&update.lock_hash)?;
    let rendezvous = validate::rendezvous(update.rendezvous)?;
    for _ in 0..storage::RETRIES {
        let revision = durable_store::revision()?;
        let Some(mut device) = registry::stored(id)? else {
            return registry::status(id, identity);
        };
        if device.identity_public_key != identity {
            return Ok(NodeStatus::Unknown);
        }
        device.hostname = hostname.clone();
        device.platform = platform.clone();
        device.arch = arch.clone();
        device.version = version.clone();
        device.capabilities = capabilities.clone();
        device.last_seen_at_ms = Some(now_ms);
        let presence = StoredPresence {
            device_id: id.into(),
            last_seen_at_ms: now_ms,
            lease_expires_at_ms,
            lock_hash: lock_hash.clone(),
            lock_generation: update.lock_generation,
            rendezvous: rendezvous.clone(),
        };
        let changes = [
            registry::device_change(&device)?,
            storage::put(PRESENCES, id.as_bytes().to_vec(), model::encode(&presence)?),
        ];
        if storage::commit(revision, &changes)? {
            return Ok(NodeStatus::Active(device.into()));
        }
    }
    Err("device state changed too frequently".into())
}

pub fn get(id: &str, now_ms: u64) -> Result<Option<Presence>, String> {
    validate::id(id, "device id")?;
    if registry::stored(id)?.is_none() {
        return Ok(None);
    }
    storage::get(PRESENCES, id.as_bytes())?
        .map(|value| model::decode::<StoredPresence>(&value).map(|value| value.view(now_ms)))
        .transpose()
}

pub fn expire(now_ms: u64) -> Result<u64, String> {
    for _ in 0..storage::RETRIES {
        let (revision, entries) = storage::scan(PRESENCES)?;
        let expired = entries
            .into_iter()
            .map(|entry| {
                model::decode::<StoredPresence>(&entry.value).map(|value| (entry.key, value))
            })
            .collect::<Result<Vec<_>, _>>()?
            .into_iter()
            .filter(|(_, value)| value.lease_expires_at_ms <= now_ms)
            .map(|(key, _)| storage::delete(PRESENCES, key))
            .collect::<Vec<_>>();
        if expired.is_empty() {
            return Ok(0);
        }
        let count = expired.len() as u64;
        if storage::commit(revision, &expired)? {
            return Ok(count);
        }
    }
    Err("device state changed too frequently".into())
}

fn validate_lock_hash(value: &str) -> Result<String, String> {
    if !value.is_empty() && value.len() <= 128 && value.bytes().all(|byte| byte.is_ascii_hexdigit())
    {
        Ok(value.to_ascii_lowercase())
    } else {
        Err("invalid lock hash".into())
    }
}
