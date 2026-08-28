use crate::{ohrats::rc_identity::types::User, storage, time, validate};
use serde::{Deserialize, Serialize};

const BUCKET: &str = "users";

#[derive(Serialize, Deserialize)]
struct StoredUser {
    id: String,
    display_name: String,
    created_at_ms: u64,
}

pub fn create(id: String, display_name: String) -> Result<User, String> {
    validate::id(&id, "user id")?;
    validate::display_name(&display_name)?;
    let stored = StoredUser {
        id,
        display_name,
        created_at_ms: time::now_ms(),
    };
    let key = stored.id.as_bytes().to_vec();
    let value = serde_json::to_vec(&stored).map_err(display)?;
    storage::insert(BUCKET, &key, value)?;
    Ok(stored.into())
}

pub fn get(id: &str) -> Result<Option<User>, String> {
    validate::id(id, "user id")?;
    storage::get(BUCKET, id.as_bytes())?
        .map(|value| serde_json::from_slice::<StoredUser>(&value).map(Into::into))
        .transpose()
        .map_err(display)
}

pub fn count() -> Result<u64, String> {
    u64::try_from(storage::scan_all(BUCKET)?.len())
        .map_err(|_| "user count exceeds supported range".into())
}

impl From<StoredUser> for User {
    fn from(value: StoredUser) -> Self {
        Self {
            id: value.id,
            display_name: value.display_name,
            created_at_ms: value.created_at_ms,
        }
    }
}

fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}
