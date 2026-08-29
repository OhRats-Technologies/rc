use crate::model::{self, StoredState};
use crate::ohrats::rc_storage::durable_store;
use crate::ohrats::rc_storage::types::{Change, CommitError, Entry};

const BUCKET: &str = "authority-lock";
const KEY: &[u8] = b"state";
const RETRIES: usize = 16;

pub fn load() -> Result<Option<StoredState>, String> {
    let Some(bytes) = durable_store::get(BUCKET, KEY)? else {
        return Ok(None);
    };
    let value: StoredState =
        serde_json::from_slice(&bytes).map_err(|_| "authority lock state is corrupt")?;
    if value.hash != model::snapshot_hash(&value.snapshot) {
        return Err("authority lock hash does not match snapshot".into());
    }
    Ok(Some(value))
}

pub fn create(value: &StoredState) -> Result<bool, String> {
    for _ in 0..RETRIES {
        let revision = durable_store::revision()?;
        if load()?.is_some() {
            return Ok(false);
        }
        match durable_store::commit(revision, &[put(value)?]) {
            Ok(_) => return Ok(true),
            Err(CommitError::Conflict(_)) => continue,
            Err(CommitError::Failure(error)) => return Err(error),
        }
    }
    Err("authority state changed too frequently".into())
}

pub fn replace(
    parent_hash: &str,
    parent_generation: u64,
    value: &StoredState,
) -> Result<(), String> {
    for _ in 0..RETRIES {
        let revision = durable_store::revision()?;
        let current = load()?.ok_or_else(|| "authority lock is not initialized".to_owned())?;
        if current.hash != parent_hash || current.generation != parent_generation {
            return Err("stale authority parent".into());
        }
        match durable_store::commit(revision, &[put(value)?]) {
            Ok(_) => return Ok(()),
            Err(CommitError::Conflict(_)) => continue,
            Err(CommitError::Failure(error)) => return Err(error),
        }
    }
    Err("authority state changed too frequently".into())
}

pub fn clear_signal(value: &StoredState) -> Result<(), String> {
    let revision = durable_store::revision()?;
    durable_store::commit(revision, &[put(value)?])
        .map(|_| ())
        .map_err(commit_error)
}

fn put(value: &StoredState) -> Result<Change, String> {
    Ok(Change::Put(Entry {
        bucket: BUCKET.into(),
        key: KEY.to_vec(),
        value: serde_json::to_vec(value).map_err(|_| "authority state could not be encoded")?,
    }))
}

fn commit_error(error: CommitError) -> String {
    match error {
        CommitError::Conflict(_) => "authority state changed too frequently".into(),
        CommitError::Failure(error) => error,
    }
}
