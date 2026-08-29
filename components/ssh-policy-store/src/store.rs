use crate::ohrats::rc_storage::{
    durable_store,
    types::{Change, CommitError, Deletion, Entry},
};

const RETRIES: usize = 8;
pub const BUCKET: &str = "ssh-public-keys";

pub fn get(key: &[u8]) -> Result<Option<Vec<u8>>, String> {
    durable_store::get(BUCKET, key)
}

pub fn scan() -> Result<Vec<Entry>, String> {
    let mut after = None;
    let mut entries = Vec::new();
    loop {
        let page = durable_store::scan(BUCKET, &[], after.as_deref(), 1000)?;
        entries.extend(page.entries);
        match page.next_key {
            Some(next) => after = Some(next),
            None => return Ok(entries),
        }
    }
}

pub fn insert(key: Vec<u8>, value: Vec<u8>, fingerprint: &str) -> Result<(), String> {
    for _ in 0..RETRIES {
        let revision = durable_store::revision()?;
        if get(&key)?.is_some() {
            return Err("SSH key already registered".into());
        }
        for entry in scan()? {
            let stored: crate::key::StoredKey =
                serde_json::from_slice(&entry.value).map_err(|_| "invalid stored SSH key")?;
            if stored.fingerprint == fingerprint {
                return Err("SSH key already registered".into());
            }
        }
        let change = Change::Put(Entry {
            bucket: BUCKET.into(),
            key: key.clone(),
            value: value.clone(),
        });
        match durable_store::commit(revision, &[change]) {
            Ok(_) => return Ok(()),
            Err(CommitError::Conflict(_)) => continue,
            Err(CommitError::Failure(error)) => return Err(error),
        }
    }
    Err("SSH key state changed too frequently".into())
}

pub fn remove(key: &[u8]) -> Result<bool, String> {
    for _ in 0..RETRIES {
        let revision = durable_store::revision()?;
        if get(key)?.is_none() {
            return Ok(false);
        }
        let change = Change::Delete(Deletion {
            bucket: BUCKET.into(),
            key: key.to_vec(),
        });
        match durable_store::commit(revision, &[change]) {
            Ok(_) => return Ok(true),
            Err(CommitError::Conflict(_)) => continue,
            Err(CommitError::Failure(error)) => return Err(error),
        }
    }
    Err("SSH key state changed too frequently".into())
}
