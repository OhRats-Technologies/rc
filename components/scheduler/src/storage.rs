use crate::{
    component::ohrats::{
        rc_scheduler::types::Definition,
        rc_storage::{
            durable_store,
            types::{Change, CommitError, Deletion, Entry},
        },
    },
    model::{StoredCursor, StoredDefinition},
};

const BUCKET: &str = "schedules";
const CURSOR_BUCKET: &str = "schedule-cursors";
const RETRIES: usize = 12;

pub fn list() -> Result<Vec<Definition>, String> {
    for _ in 0..RETRIES {
        let revision = durable_store::revision()?;
        let mut after = None;
        let mut result = Vec::new();
        loop {
            let page = durable_store::scan(BUCKET, &[], after.as_deref(), 1_000)?;
            if page.revision != revision {
                break;
            }
            for entry in page.entries {
                result.push(decode(&entry.value)?);
            }
            match page.next_key {
                Some(value) => after = Some(value),
                None => {
                    result.sort_by(|left, right| left.id.cmp(&right.id));
                    return Ok(result);
                }
            }
        }
    }
    Err("schedule state changed too frequently".into())
}

pub fn get(id: &str) -> Result<Option<Definition>, String> {
    durable_store::get(BUCKET, id.as_bytes())
        .and_then(|value| value.map(|bytes| decode(&bytes)).transpose())
}

pub fn put(value: Definition) -> Result<(), String> {
    let key = value.id.as_bytes().to_vec();
    let encoded = serde_json::to_vec(&StoredDefinition::from(value)).map_err(display)?;
    commit(|_| {
        vec![Change::Put(Entry {
            bucket: BUCKET.into(),
            key: key.clone(),
            value: encoded.clone(),
        })]
    })
}

pub fn remove(id: &str) -> Result<bool, String> {
    if durable_store::get(BUCKET, id.as_bytes())?.is_none() {
        return Ok(false);
    }
    let key = id.as_bytes().to_vec();
    commit(|_| {
        vec![
            Change::Delete(Deletion {
                bucket: BUCKET.into(),
                key: key.clone(),
            }),
            Change::Delete(Deletion {
                bucket: CURSOR_BUCKET.into(),
                key: key.clone(),
            }),
        ]
    })?;
    Ok(true)
}

pub fn cursor(id: &str) -> Result<Option<StoredCursor>, String> {
    durable_store::get(CURSOR_BUCKET, id.as_bytes())?
        .map(|bytes| serde_json::from_slice(&bytes).map_err(display))
        .transpose()
}

pub fn put_cursor(id: &str, value: &StoredCursor) -> Result<(), String> {
    let key = id.as_bytes().to_vec();
    let encoded = serde_json::to_vec(value).map_err(display)?;
    commit(|_| {
        vec![Change::Put(Entry {
            bucket: CURSOR_BUCKET.into(),
            key: key.clone(),
            value: encoded.clone(),
        })]
    })
}

fn decode(bytes: &[u8]) -> Result<Definition, String> {
    serde_json::from_slice::<StoredDefinition>(bytes)
        .map_err(display)?
        .try_into()
}

fn commit(mut changes: impl FnMut(u64) -> Vec<Change>) -> Result<(), String> {
    for _ in 0..RETRIES {
        let revision = durable_store::revision()?;
        match durable_store::commit(revision, &changes(revision)) {
            Ok(_) => return Ok(()),
            Err(CommitError::Conflict(_)) => continue,
            Err(CommitError::Failure(error)) => return Err(error),
        }
    }
    Err("schedule state changed too frequently".into())
}

fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}
