use crate::ohrats::rc_storage::{
    durable_store,
    types::{Change, CommitError, Deletion, Entry},
};

pub const EVENTS: &str = "lifecycle-events";
pub const IDEMPOTENCY: &str = "event-idempotency";
pub const META: &str = "event-meta";
const RETRIES: usize = 12;

pub fn get(bucket: &str, key: &[u8]) -> Result<Option<Vec<u8>>, String> {
    durable_store::get(bucket, key)
}

pub fn scan(bucket: &str) -> Result<Vec<Entry>, String> {
    for _ in 0..RETRIES {
        let expected = durable_store::revision()?;
        let mut entries = Vec::new();
        let mut after = None;
        loop {
            let page = durable_store::scan(bucket, &[], after.as_deref(), 1000)?;
            if page.revision != expected {
                break;
            }
            entries.extend(page.entries);
            match page.next_key {
                Some(key) => after = Some(key),
                None => return Ok(entries),
            }
        }
    }
    Err("event state changed too frequently".into())
}

pub fn transact(mut build: impl FnMut(u64) -> Result<Vec<Change>, String>) -> Result<(), String> {
    for _ in 0..RETRIES {
        let revision = durable_store::revision()?;
        let changes = build(revision)?;
        match durable_store::commit(revision, &changes) {
            Ok(_) => return Ok(()),
            Err(CommitError::Conflict(_)) => continue,
            Err(CommitError::Failure(error)) => return Err(error),
        }
    }
    Err("event state changed too frequently".into())
}

pub fn put(bucket: &str, key: Vec<u8>, value: Vec<u8>) -> Change {
    Change::Put(Entry {
        bucket: bucket.into(),
        key,
        value,
    })
}

pub fn delete(bucket: &str, key: Vec<u8>) -> Change {
    Change::Delete(Deletion {
        bucket: bucket.into(),
        key,
    })
}

pub fn cursor_key(cursor: u64) -> Vec<u8> {
    cursor.to_be_bytes().to_vec()
}
pub fn read_u64(value: Option<Vec<u8>>) -> Result<u64, String> {
    let bytes: [u8; 8] = value
        .unwrap_or_else(|| 0u64.to_be_bytes().to_vec())
        .try_into()
        .map_err(|_| "invalid event metadata".to_owned())?;
    Ok(u64::from_be_bytes(bytes))
}
