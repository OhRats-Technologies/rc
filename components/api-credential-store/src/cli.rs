use crate::ohrats::rc_api_credentials::types::{
    Administrator, CliAuthorization, Credential, Lifetime, Scope,
};
use crate::{
    admin, crypto,
    model::{self, PendingCli, StoredCredential},
    storage, validate,
};
const REQUEST_TTL_MS: u64 = 10 * 60 * 1000;

pub fn start(
    client_id: String,
    public_key: String,
    lifetime: Option<Lifetime>,
    request_id: String,
    device_code: String,
    user_code: String,
    now_ms: u64,
) -> Result<CliAuthorization, String> {
    validate::id(&client_id, "CLI client id")?;
    validate::id(&request_id, "CLI request id")?;
    validate::public_key(&public_key)?;
    validate::text(&device_code, "CLI device code", validate::MAX_CODE)?;
    validate::text(&user_code, "CLI user code", validate::MAX_CODE)?;
    if storage::get(storage::AUTHORIZATIONS, request_id.as_bytes())?.is_some() {
        return Err("CLI authorization already exists".into());
    }
    let pending = PendingCli {
        request_id: request_id.clone(),
        client_id,
        public_key,
        lifetime: model::lifetime(lifetime),
        device_code_hash: crypto::hash(&device_code),
        user_code_hash: crypto::hash(&user_code),
        created_at_ms: now_ms,
        expires_at_ms: now_ms.saturating_add(REQUEST_TTL_MS),
        approved_at_ms: None,
        exchanged_at_ms: None,
    };
    storage::put(
        storage::AUTHORIZATIONS,
        request_id.as_bytes().to_vec(),
        serde_json::to_vec(&pending).map_err(|_| "encode CLI authorization")?,
    )?;
    Ok(CliAuthorization {
        request_id,
        client_id: pending.client_id,
        expires_at_ms: pending.expires_at_ms,
        interval_seconds: 2,
        verification_url: format!("/cli/login?code={user_code}"),
    })
}

pub fn approve(
    admin_value: Administrator,
    request_id: &str,
    user_code: &str,
    browser_public_key: &str,
) -> Result<Credential, String> {
    admin::check(&admin_value)?;
    let mut pending = load(request_id)?.ok_or_else(|| "CLI authorization expired".to_owned())?;
    if pending.expires_at_ms <= admin_value.now_ms
        || pending.exchanged_at_ms.is_some()
        || pending.approved_at_ms.is_some()
        || pending.user_code_hash != crypto::hash(user_code)
        || pending.client_id != admin_value.browser_client_id
        || pending.public_key != browser_public_key
    {
        return Err("CLI authorization expired".into());
    }
    let id = pending.client_id.clone();
    if storage::get(storage::CREDENTIALS, id.as_bytes())?.is_some() {
        return Err("CLI credential already exists".into());
    }
    let value = StoredCredential {
        id: id.clone(),
        user_id: admin_value.user_id,
        kind: 1,
        name: "RC CLI".into(),
        public_key: pending.public_key.clone(),
        scopes: vec![model::scope(Scope::Read), model::scope(Scope::Execute)],
        created_at_ms: admin_value.now_ms,
        expires_at_ms: crate::validate::lifetime(
            pending.lifetime.map(|value| match value {
                1 => Lifetime::OneHour,
                2 => Lifetime::OneDay,
                3 => Lifetime::SevenDays,
                4 => Lifetime::ThirtyDays,
                5 => Lifetime::NinetyDays,
                6 => Lifetime::OneEightyDays,
                7 => Lifetime::OneYear,
                _ => Lifetime::Never,
            }),
            admin_value.now_ms,
        ),
        last_used_at_ms: None,
        revoked_at_ms: None,
    };
    pending.approved_at_ms = Some(admin_value.now_ms);
    storage::put(
        storage::CREDENTIALS,
        id.into_bytes(),
        serde_json::to_vec(&value).map_err(|_| "encode credential")?,
    )?;
    storage::put(
        storage::AUTHORIZATIONS,
        request_id.as_bytes().to_vec(),
        serde_json::to_vec(&pending).map_err(|_| "encode CLI authorization")?,
    )?;
    Ok(value.output())
}

pub fn poll(
    request_id: &str,
    device_code: &str,
    now_ms: u64,
) -> Result<Option<Credential>, String> {
    let Some(mut pending) = load(request_id)? else {
        return Err("CLI authorization expired".into());
    };
    if pending.expires_at_ms <= now_ms
        || pending.device_code_hash != crypto::hash(device_code)
        || pending.exchanged_at_ms.is_some()
    {
        return Err("CLI authorization expired".into());
    }
    let Some(_) = pending.approved_at_ms else {
        return Ok(None);
    };
    pending.exchanged_at_ms = Some(now_ms);
    let credential = crate::credentials::get(&pending.client_id)?
        .ok_or_else(|| "CLI credential expired".to_owned())?;
    storage::put(
        storage::AUTHORIZATIONS,
        request_id.as_bytes().to_vec(),
        serde_json::to_vec(&pending).map_err(|_| "encode CLI authorization")?,
    )?;
    Ok(Some(credential))
}

pub fn revoke(admin_value: Administrator, id: &str) -> Result<bool, String> {
    crate::credentials::revoke(admin_value, id)
}

fn load(id: &str) -> Result<Option<PendingCli>, String> {
    validate::id(id, "CLI request id")?;
    storage::get(storage::AUTHORIZATIONS, id.as_bytes())?
        .map(|value| serde_json::from_slice(&value).map_err(|_| "invalid CLI authorization".into()))
        .transpose()
}
