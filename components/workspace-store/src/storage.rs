use crate::ohrats::rc_storage::{
    durable_store,
    types::{Change, CommitError, Deletion, Entry},
};

const RETRIES: usize = 16;
const PAGE_SIZE: u32 = 256;

pub fn get(bucket: &str, key: &[u8]) -> Result<Option<Vec<u8>>, String> {
    durable_store::get(bucket, key)
}

pub fn commit<T, F>(mut prepare: F) -> Result<T, String>
where
    F: FnMut() -> Result<(T, Vec<Change>), String>,
{
    for _ in 0..RETRIES {
        let revision = durable_store::revision()?;
        let (result, changes) = prepare()?;
        if changes.is_empty() {
            return Ok(result);
        }
        match durable_store::commit(revision, &changes) {
            Ok(_) => return Ok(result),
            Err(CommitError::Conflict(_)) => continue,
            Err(CommitError::Failure(error)) => return Err(error),
        }
    }
    Err("workspace state changed too frequently".into())
}

pub fn scan(bucket: &str, prefix: &[u8], limit: usize) -> Result<Vec<Entry>, String> {
    for _ in 0..RETRIES {
        let expected = durable_store::revision()?;
        let mut entries = Vec::new();
        let mut after = None;
        loop {
            let page = durable_store::scan(bucket, prefix, after.as_deref(), PAGE_SIZE)?;
            if page.revision != expected {
                break;
            }
            entries.extend(page.entries);
            if entries.len() > limit {
                return Err("workspace state exceeds its capacity".into());
            }
            match page.next_key {
                Some(next) => after = Some(next),
                None => return Ok(entries),
            }
        }
    }
    Err("workspace state changed too frequently".into())
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
