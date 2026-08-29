use crate::ohrats::rc_api_credentials::types::{
    Administrator, Credential, Kind, Lifetime, Request, Scope, Verified,
};
use crate::{
    admin, crypto,
    model::{self, StoredCredential},
    storage, validate,
};

pub fn create_api(
    admin_value: Administrator,
    id: String,
    name: String,
    public_key: String,
    requested: Vec<Scope>,
    lifetime: Option<Lifetime>,
) -> Result<Credential, String> {
    admin::check(&admin_value)?;
    validate::id(&id, "credential id")?;
    validate::text(&name, "credential name", validate::MAX_NAME)?;
    validate::public_key(&public_key)?;
    let scopes = validate::scopes(&requested);
    let existing = storage::get(storage::CREDENTIALS, id.as_bytes())?;
    if existing.is_some() {
        return Err("API credential already exists".into());
    }
    let count = list(&admin_value.user_id)?
        .into_iter()
        .filter(|v| v.kind == Kind::Api)
        .count();
    if count >= 10 {
        return Err("API key limit reached (10)".into());
    }
    let value = StoredCredential {
        id: id.clone(),
        user_id: admin_value.user_id,
        kind: 0,
        name,
        public_key,
        scopes: scopes.into_iter().map(model::scope).collect(),
        created_at_ms: admin_value.now_ms,
        expires_at_ms: validate::lifetime(lifetime, admin_value.now_ms),
        last_used_at_ms: None,
        revoked_at_ms: None,
    };
    storage::put(
        storage::CREDENTIALS,
        id.into_bytes(),
        serde_json::to_vec(&value).map_err(|_| "encode credential")?,
    )?;
    Ok(value.output())
}

pub fn list(user_id: &str) -> Result<Vec<Credential>, String> {
    validate::id(user_id, "user id")?;
    let mut values = storage::scan(storage::CREDENTIALS)?
        .into_iter()
        .filter_map(|entry| serde_json::from_slice::<StoredCredential>(&entry.value).ok())
        .filter(|value| value.user_id == user_id)
        .map(|value| value.output())
        .collect::<Vec<_>>();
    values.sort_by(|a, b| a.id.cmp(&b.id));
    Ok(values)
}

pub fn get(id: &str) -> Result<Option<Credential>, String> {
    validate::id(id, "credential id")?;
    Ok(storage::get(storage::CREDENTIALS, id.as_bytes())?
        .map(|value| {
            serde_json::from_slice::<StoredCredential>(&value)
                .map(|value| value.output())
                .map_err(|_| "invalid credential")
        })
        .transpose()?)
}

pub fn revoke(admin_value: Administrator, id: &str) -> Result<bool, String> {
    admin::check(&admin_value)?;
    let Some(mut value) = load(id)? else {
        return Ok(false);
    };
    if value.user_id != admin_value.user_id || value.revoked_at_ms.is_some() {
        return Ok(false);
    }
    value.revoked_at_ms = Some(admin_value.now_ms);
    storage::put(
        storage::CREDENTIALS,
        id.as_bytes().to_vec(),
        serde_json::to_vec(&value).map_err(|_| "encode credential")?,
    )?;
    Ok(true)
}

pub fn verify(request: Request, now_ms: u64) -> Result<Verified, String> {
    let timestamp = request
        .timestamp_seconds
        .parse::<i64>()
        .map_err(|_| "invalid timestamp")?;
    let now = i64::try_from(now_ms / 1000).map_err(|_| "invalid clock")?;
    if (now - timestamp).abs() > 60 {
        return Err("expired client authentication".into());
    }
    let nonce_key = format!("{}:{}", request.key_id, hex(&crypto::hash(&request.nonce)));
    for _ in 0..8 {
        // Capture the revision before every read. If another verifier records
        // this nonce while we validate, our commit must conflict and retry the
        // nonce lookup instead of accepting the stale absence.
        let revision = crate::ohrats::rc_storage::durable_store::revision()?;
        let mut value =
            load(&request.key_id)?.ok_or_else(|| "client authentication rejected".to_owned())?;
        if value.revoked_at_ms.is_some()
            || (value.expires_at_ms != 0 && value.expires_at_ms <= now_ms)
        {
            return Err("client authentication rejected".into());
        }
        crypto::verify(&request, &value.public_key)?;
        let nonce_value = storage::get(storage::NONCES, nonce_key.as_bytes())?;
        if nonce_value
            .as_deref()
            .and_then(nonce_expiry)
            .is_some_and(|expires| expires > now_ms)
        {
            return Err("replayed client request".into());
        }
        value.last_used_at_ms = Some(now_ms);
        let mut changes = expired_nonce_deletions(now_ms, &nonce_key)?;
        use crate::ohrats::rc_storage::types::{Change, Entry};
        changes.extend([
            Change::Put(Entry {
                bucket: storage::CREDENTIALS.into(),
                key: request.key_id.as_bytes().to_vec(),
                value: serde_json::to_vec(&value).map_err(|_| "encode credential")?,
            }),
            Change::Put(Entry {
                bucket: storage::NONCES.into(),
                key: nonce_key.as_bytes().to_vec(),
                value: (now_ms + 120_000).to_string().into_bytes(),
            }),
        ]);
        match storage::commit_once(revision, &changes) {
            Ok(()) => {
                let output = value.output();
                return Ok(Verified {
                    credential_id: output.id,
                    user_id: output.user_id,
                    kind: output.kind,
                    scopes: output.scopes,
                });
            }
            Err(crate::ohrats::rc_storage::types::CommitError::Conflict(_)) => continue,
            Err(crate::ohrats::rc_storage::types::CommitError::Failure(error)) => {
                return Err(error);
            }
        }
    }
    Err("API credential state changed too frequently".into())
}

fn nonce_expiry(value: &[u8]) -> Option<u64> {
    std::str::from_utf8(value).ok()?.parse().ok()
}

fn expired_nonce_deletions(
    now_ms: u64,
    current: &str,
) -> Result<Vec<crate::ohrats::rc_storage::types::Change>, String> {
    Ok(storage::scan_limit(storage::NONCES, 128)?
        .into_iter()
        .filter_map(|entry| {
            (entry.key != current.as_bytes()
                && nonce_expiry(&entry.value).is_some_and(|expires| expires <= now_ms))
            .then(|| storage::delete(storage::NONCES, entry.key))
        })
        .collect())
}

fn load(id: &str) -> Result<Option<StoredCredential>, String> {
    validate::id(id, "credential id")?;
    storage::get(storage::CREDENTIALS, id.as_bytes())?
        .map(|value| serde_json::from_slice(&value).map_err(|_| "invalid credential".into()))
        .transpose()
}

fn hex(bytes: &[u8]) -> String {
    bytes.iter().map(|b| format!("{b:02x}")).collect()
}
