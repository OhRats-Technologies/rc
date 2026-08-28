use crate::ohrats::rc_storage::{
    durable_store,
    types::{Change, CommitError, Entry},
};

pub const RETRIES: usize = 8;
const PAGE_SIZE: u32 = 1000;

pub fn get(bucket: &str, key: &[u8]) -> Result<Option<Vec<u8>>, String> {
    durable_store::get(bucket, key)
}

pub fn scan(bucket: &str) -> Result<(u64, Vec<Entry>), String> {
    for _ in 0..RETRIES {
        let revision = durable_store::revision()?;
        let mut after = None;
        let mut entries = Vec::new();
        loop {
            let page = durable_store::scan(bucket, &[], after.as_deref(), PAGE_SIZE)?;
            if page.revision != revision {
                break;
            }
            entries.extend(page.entries);
            let Some(next) = page.next_key else {
                return Ok((revision, entries));
            };
            after = Some(next);
        }
    }
    Err("device state changed too frequently".into())
}

pub fn commit(revision: u64, changes: &[Change]) -> Result<bool, String> {
    match durable_store::commit(revision, changes) {
        Ok(_) => Ok(true),
        Err(CommitError::Conflict(_)) => Ok(false),
        Err(CommitError::Failure(error)) => Err(error),
    }
}

pub fn put(bucket: &str, key: Vec<u8>, value: Vec<u8>) -> Change {
    Change::Put(Entry {
        bucket: bucket.into(),
        key,
        value,
    })
}

pub fn delete(bucket: &str, key: Vec<u8>) -> Change {
    Change::Delete(crate::ohrats::rc_storage::types::Deletion {
        bucket: bucket.into(),
        key,
    })
}
