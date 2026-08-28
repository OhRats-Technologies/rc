use crate::{
    exports::ohrats::rc_identity::credentials::Passkey,
    ohrats::rc_identity::types::User,
    ohrats::rc_webauthn::types::StoredCredential,
    storage::{self, Delete, Put},
    time, user, validate,
};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

const BUCKET: &str = "passkeys";
const INDEX: &str = "passkeys-by-credential";
const ID_BYTES: usize = 16;

#[derive(Serialize, Deserialize)]
struct StoredPasskey {
    id: String,
    user_id: String,
    name: String,
    credential: CredentialData,
    created_at_ms: u64,
    last_used_at_ms: Option<u64>,
}

#[derive(Serialize, Deserialize)]
struct CredentialData {
    id: Vec<u8>,
    algorithm: String,
    public_key_cose: Vec<u8>,
    sign_count: u32,
    backup_eligible: bool,
    backup_state: bool,
}

pub fn create_user(
    user_id: String,
    display_name: String,
    passkey_name: String,
    credential: StoredCredential,
) -> Result<User, String> {
    let (user, user_key, user_value) = user::prepare(user_id.clone(), display_name)?;
    let (passkey, passkey_key, passkey_value, index_key) =
        prepare(user_id, passkey_name, credential)?;
    storage::insert_many(vec![
        Put {
            bucket: user::BUCKET.into(),
            key: user_key,
            value: user_value,
        },
        Put {
            bucket: BUCKET.into(),
            key: passkey_key,
            value: passkey_value,
        },
        Put {
            bucket: INDEX.into(),
            key: index_key,
            value: passkey.id.as_bytes().to_vec(),
        },
    ])?;
    Ok(user)
}

pub fn add(user_id: String, name: String, credential: StoredCredential) -> Result<Passkey, String> {
    if user::get(&user_id)?.is_none() {
        return Err("user not found".into());
    }
    let (passkey, key, value, index_key) = prepare(user_id, name, credential)?;
    storage::insert_many(vec![
        Put {
            bucket: BUCKET.into(),
            key,
            value,
        },
        Put {
            bucket: INDEX.into(),
            key: index_key,
            value: passkey.id.as_bytes().to_vec(),
        },
    ])?;
    Ok(passkey)
}

pub fn get_by_credential_id(id: &[u8]) -> Result<Option<Passkey>, String> {
    validate::stored_credential(id, "es256", &[1])?;
    let Some(internal) = storage::get(INDEX, &credential_key(id))? else {
        return Ok(None);
    };
    let internal = String::from_utf8(internal).map_err(display)?;
    get(&internal)
}

pub fn all(user_id: Option<&str>) -> Result<Vec<Passkey>, String> {
    if let Some(user_id) = user_id {
        validate::id(user_id, "user id")?;
    }
    storage::scan_all(BUCKET)?
        .into_iter()
        .map(|entry| serde_json::from_slice::<StoredPasskey>(&entry.value).map_err(display))
        .filter_map(|value| match value {
            Ok(value) if user_id.is_none_or(|id| id == value.user_id) => Some(Ok(value.into())),
            Ok(_) => None,
            Err(error) => Some(Err(error)),
        })
        .collect()
}

pub fn update(id: &str, credential: StoredCredential, used_at_ms: u64) -> Result<Passkey, String> {
    validate::id(id, "passkey id")?;
    validate_credential(&credential)?;
    let Some(value) = storage::get(BUCKET, id.as_bytes())? else {
        return Err("passkey not found".into());
    };
    let mut stored: StoredPasskey = serde_json::from_slice(&value).map_err(display)?;
    if stored.credential.id != credential.id {
        return Err("WebAuthn credential id cannot change".into());
    }
    stored.credential = credential.into();
    stored.last_used_at_ms = Some(used_at_ms.max(time::now_ms()));
    storage::replace(
        BUCKET,
        id.as_bytes(),
        serde_json::to_vec(&stored).map_err(display)?,
    )?;
    Ok(stored.into())
}

pub fn remove(id: &str, user_id: &str) -> Result<bool, String> {
    validate::id(id, "passkey id")?;
    validate::id(user_id, "user id")?;
    let Some(value) = storage::get(BUCKET, id.as_bytes())? else {
        return Ok(false);
    };
    let stored: StoredPasskey = serde_json::from_slice(&value).map_err(display)?;
    if stored.user_id != user_id {
        return Ok(false);
    }
    storage::remove_many(vec![
        Delete {
            bucket: BUCKET.into(),
            key: id.as_bytes().to_vec(),
        },
        Delete {
            bucket: INDEX.into(),
            key: credential_key(&stored.credential.id),
        },
    ])
}

fn get(id: &str) -> Result<Option<Passkey>, String> {
    validate::id(id, "passkey id")?;
    storage::get(BUCKET, id.as_bytes())?
        .map(|value| serde_json::from_slice::<StoredPasskey>(&value).map(Into::into))
        .transpose()
        .map_err(display)
}

fn prepare(
    user_id: String,
    name: String,
    credential: StoredCredential,
) -> Result<(Passkey, Vec<u8>, Vec<u8>, Vec<u8>), String> {
    validate::id(&user_id, "user id")?;
    validate::passkey_name(&name)?;
    validate_credential(&credential)?;
    let stored = StoredPasskey {
        id: random_id()?,
        user_id,
        name,
        credential: credential.into(),
        created_at_ms: time::now_ms(),
        last_used_at_ms: None,
    };
    let key = stored.id.as_bytes().to_vec();
    let index = credential_key(&stored.credential.id);
    let value = serde_json::to_vec(&stored).map_err(display)?;
    Ok((stored.into(), key, value, index))
}

fn validate_credential(value: &StoredCredential) -> Result<(), String> {
    validate::stored_credential(&value.id, &value.algorithm, &value.public_key_cose)
}

fn credential_key(id: &[u8]) -> Vec<u8> {
    Sha256::digest(id).to_vec()
}

fn random_id() -> Result<String, String> {
    let mut bytes = [0; ID_BYTES];
    getrandom::fill(&mut bytes).map_err(display)?;
    Ok(URL_SAFE_NO_PAD.encode(bytes))
}

impl From<CredentialData> for StoredCredential {
    fn from(value: CredentialData) -> Self {
        Self {
            id: value.id,
            algorithm: value.algorithm,
            public_key_cose: value.public_key_cose,
            sign_count: value.sign_count,
            backup_eligible: value.backup_eligible,
            backup_state: value.backup_state,
        }
    }
}

impl From<StoredCredential> for CredentialData {
    fn from(value: StoredCredential) -> Self {
        Self {
            id: value.id,
            algorithm: value.algorithm,
            public_key_cose: value.public_key_cose,
            sign_count: value.sign_count,
            backup_eligible: value.backup_eligible,
            backup_state: value.backup_state,
        }
    }
}

impl From<StoredPasskey> for Passkey {
    fn from(value: StoredPasskey) -> Self {
        Self {
            id: value.id,
            user_id: value.user_id,
            name: value.name,
            credential: value.credential.into(),
            created_at_ms: value.created_at_ms,
            last_used_at_ms: value.last_used_at_ms,
        }
    }
}

fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}
