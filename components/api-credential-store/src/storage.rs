use crate::ohrats::rc_storage::{
    durable_store,
    types::{Change, CommitError, Deletion, Entry},
};

pub const CREDENTIALS: &str = "api-credentials";
pub const AUTHORIZATIONS: &str = "api-cli-authorizations";
pub const NONCES: &str = "api-request-nonces";

pub fn get(bucket: &str, key: &[u8]) -> Result<Option<Vec<u8>>, String> {
    durable_store::get(bucket, key)
}

pub fn put(bucket: &str, key: Vec<u8>, value: Vec<u8>) -> Result<(), String> {
    for _ in 0..8 {
        let revision = durable_store::revision()?;
        let change = Change::Put(Entry {
            bucket: bucket.into(),
            key: key.clone(),
            value: value.clone(),
        });
        match durable_store::commit(revision, &[change]) {
            Ok(_) => return Ok(()),
            Err(CommitError::Conflict(_)) => continue,
            Err(CommitError::Failure(error)) => return Err(error),
        }
    }
    Err("API credential state changed too frequently".into())
}

pub fn commit_once(expected: u64, changes: &[Change]) -> Result<(), CommitError> {
    durable_store::commit(expected, changes).map(|_| ())
}

pub fn scan(bucket: &str) -> Result<Vec<Entry>, String> {
    let mut entries = Vec::new();
    let mut after = None;
    loop {
        let page = durable_store::scan(bucket, &[], after.as_deref(), 1000)?;
        entries.extend(page.entries);
        let Some(next) = page.next_key else {
            return Ok(entries);
        };
        after = Some(next);
    }
}

pub fn scan_limit(bucket: &str, limit: u32) -> Result<Vec<Entry>, String> {
    Ok(durable_store::scan(bucket, &[], None, limit)?.entries)
}

pub fn delete(bucket: &str, key: Vec<u8>) -> Change {
    Change::Delete(Deletion {
        bucket: bucket.into(),
        key,
    })
}
