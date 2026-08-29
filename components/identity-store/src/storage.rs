use crate::ohrats::rc_storage::{
    durable_store,
    types::{Change, CommitError, Deletion, Entry},
};

const RETRIES: usize = 8;
const PAGE_SIZE: u32 = 1000;

pub struct Put {
    pub bucket: String,
    pub key: Vec<u8>,
    pub value: Vec<u8>,
}

pub struct Delete {
    pub bucket: String,
    pub key: Vec<u8>,
}

pub fn get(bucket: &str, key: &[u8]) -> Result<Option<Vec<u8>>, String> {
    durable_store::get(bucket, key)
}

pub fn insert(bucket: &str, key: &[u8], value: Vec<u8>) -> Result<(), String> {
    insert_many(vec![Put {
        bucket: bucket.into(),
        key: key.to_vec(),
        value,
    }])
}

pub fn insert_many(values: Vec<Put>) -> Result<(), String> {
    for _ in 0..RETRIES {
        let revision = durable_store::revision()?;
        for value in &values {
            if durable_store::get(&value.bucket, &value.key)?.is_some() {
                return Err("identity record already exists".into());
            }
        }
        let changes = values
            .iter()
            .map(|value| {
                Change::Put(Entry {
                    bucket: value.bucket.clone(),
                    key: value.key.clone(),
                    value: value.value.clone(),
                })
            })
            .collect::<Vec<_>>();
        match durable_store::commit(revision, &changes) {
            Ok(_) => return Ok(()),
            Err(CommitError::Conflict(_)) => continue,
            Err(CommitError::Failure(error)) => return Err(error),
        }
    }
    Err("identity state changed too frequently".into())
}

pub fn replace(bucket: &str, key: &[u8], value: Vec<u8>) -> Result<(), String> {
    for _ in 0..RETRIES {
        let revision = durable_store::revision()?;
        if durable_store::get(bucket, key)?.is_none() {
            return Err("identity record not found".into());
        }
        let change = Change::Put(Entry {
            bucket: bucket.into(),
            key: key.to_vec(),
            value: value.clone(),
        });
        match durable_store::commit(revision, &[change]) {
            Ok(_) => return Ok(()),
            Err(CommitError::Conflict(_)) => continue,
            Err(CommitError::Failure(error)) => return Err(error),
        }
    }
    Err("identity state changed too frequently".into())
}

pub fn take(bucket: &str, key: &[u8]) -> Result<Option<Vec<u8>>, String> {
    for _ in 0..RETRIES {
        let revision = durable_store::revision()?;
        let Some(value) = durable_store::get(bucket, key)? else {
            return Ok(None);
        };
        let change = Change::Delete(Deletion {
            bucket: bucket.into(),
            key: key.to_vec(),
        });
        match durable_store::commit(revision, &[change]) {
            Ok(_) => return Ok(Some(value)),
            Err(CommitError::Conflict(_)) => continue,
            Err(CommitError::Failure(error)) => return Err(error),
        }
    }
    Err("identity state changed too frequently".into())
}

pub fn remove(bucket: &str, key: &[u8]) -> Result<bool, String> {
    Ok(take(bucket, key)?.is_some())
}

pub fn remove_many(values: Vec<Delete>) -> Result<bool, String> {
    for _ in 0..RETRIES {
        let revision = durable_store::revision()?;
        let mut found = false;
        for value in &values {
            found |= durable_store::get(&value.bucket, &value.key)?.is_some();
        }
        if !found {
            return Ok(false);
        }
        let changes = values
            .iter()
            .map(|value| {
                Change::Delete(Deletion {
                    bucket: value.bucket.clone(),
                    key: value.key.clone(),
                })
            })
            .collect::<Vec<_>>();
        match durable_store::commit(revision, &changes) {
            Ok(_) => return Ok(true),
            Err(CommitError::Conflict(_)) => continue,
            Err(CommitError::Failure(error)) => return Err(error),
        }
    }
    Err("identity state changed too frequently".into())
}

pub fn scan_all(bucket: &str) -> Result<Vec<Entry>, String> {
    for _ in 0..RETRIES {
        let expected = durable_store::revision()?;
        let mut entries = Vec::new();
        let mut after = None;
        let mut changed = false;
        loop {
            let page = durable_store::scan(bucket, &[], after.as_deref(), PAGE_SIZE)?;
            if page.revision != expected {
                changed = true;
                break;
            }
            entries.extend(page.entries);
            let Some(next) = page.next_key else {
                break;
            };
            after = Some(next);
        }
        if !changed {
            return Ok(entries);
        }
    }
    Err("identity state changed too frequently".into())
}
