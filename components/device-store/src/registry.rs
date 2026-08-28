use crate::{
    model::{self, DEVICES, IDENTITIES, PRESENCES, StoredDevice, StoredTombstone, TOMBSTONES},
    ohrats::{
        rc_devices::types::{Device, NodeStatus, Tombstone},
        rc_storage::types::Change,
    },
    storage, validate,
};

pub fn get(id: &str) -> Result<Option<Device>, String> {
    validate::id(id, "device id")?;
    stored(id).map(|value| value.map(Into::into))
}

pub fn stored(id: &str) -> Result<Option<StoredDevice>, String> {
    storage::get(DEVICES, id.as_bytes())?
        .map(|value| model::decode(&value))
        .transpose()
}

pub fn list(workspace: Option<&str>) -> Result<Vec<Device>, String> {
    if let Some(id) = workspace {
        validate::id(id, "workspace id")?;
    }
    let (_, entries) = storage::scan(DEVICES)?;
    entries
        .into_iter()
        .map(|entry| model::decode::<StoredDevice>(&entry.value))
        .filter_map(|value| match value {
            Ok(value) if workspace.is_none_or(|id| value.workspace_id == id) => {
                Some(Ok(value.into()))
            }
            Ok(_) => None,
            Err(error) => Some(Err(error)),
        })
        .collect()
}

pub fn rename(id: &str, name: String) -> Result<Device, String> {
    validate::id(id, "device id")?;
    let name = validate::text(&name, "device name", 120)?;
    for _ in 0..storage::RETRIES {
        let revision = crate::ohrats::rc_storage::durable_store::revision()?;
        let Some(mut device) = stored(id)? else {
            return Err("device not found".into());
        };
        device.name = name.clone();
        let change = storage::put(DEVICES, id.as_bytes().to_vec(), model::encode(&device)?);
        if storage::commit(revision, &[change])? {
            return Ok(device.into());
        }
    }
    Err("device state changed too frequently".into())
}

pub fn revoke(id: &str, revoked_at_ms: u64) -> Result<Option<Tombstone>, String> {
    validate::id(id, "device id")?;
    if revoked_at_ms == 0 {
        return Err("invalid revocation time".into());
    }
    for _ in 0..storage::RETRIES {
        let revision = crate::ohrats::rc_storage::durable_store::revision()?;
        let Some(device) = stored(id)? else {
            return tombstone(id).map(|value| value.map(Into::into));
        };
        let tombstone = StoredTombstone {
            device_id: id.into(),
            identity_public_key: device.identity_public_key,
            revoked_at_ms,
        };
        let changes = [
            storage::put(
                TOMBSTONES,
                id.as_bytes().to_vec(),
                model::encode(&tombstone)?,
            ),
            storage::delete(DEVICES, id.as_bytes().to_vec()),
            storage::delete(PRESENCES, id.as_bytes().to_vec()),
        ];
        if storage::commit(revision, &changes)? {
            return Ok(Some(tombstone.into()));
        }
    }
    Err("device state changed too frequently".into())
}

pub fn status(id: &str, identity: &str) -> Result<NodeStatus, String> {
    validate::id(id, "device id")?;
    validate::key(identity, "identity public key")?;
    if let Some(device) = stored(id)? {
        return if device.identity_public_key == identity {
            Ok(NodeStatus::Active(device.into()))
        } else {
            Ok(NodeStatus::Unknown)
        };
    }
    if let Some(value) = tombstone(id)? {
        return if value.identity_public_key == identity {
            Ok(NodeStatus::Revoked(value.into()))
        } else {
            Ok(NodeStatus::Unknown)
        };
    }
    Ok(NodeStatus::Unknown)
}

fn tombstone(id: &str) -> Result<Option<StoredTombstone>, String> {
    storage::get(TOMBSTONES, id.as_bytes())?
        .map(|value| model::decode(&value))
        .transpose()
}

pub fn identity_device(identity: &str) -> Result<Option<String>, String> {
    storage::get(IDENTITIES, identity.as_bytes())?
        .map(|value| String::from_utf8(value).map_err(|error| error.to_string()))
        .transpose()
}

pub fn device_change(device: &StoredDevice) -> Result<Change, String> {
    Ok(storage::put(
        DEVICES,
        device.id.as_bytes().to_vec(),
        model::encode(device)?,
    ))
}
