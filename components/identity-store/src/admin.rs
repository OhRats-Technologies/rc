use crate::{
    credential,
    exports::ohrats::rc_identity::{admin_consumer::Claim, admin_issuer::Challenge},
    ohrats::rc_webauthn::{
        types::{AuthenticationRequest, RelyingParty},
        verifier,
    },
    session, storage, time, validate,
};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

const CEREMONY_KIND: &str = "admin-step-up";
const AUTH_BUCKET: &str = "human-admin-authorizations";
const CEREMONY_TTL_MS: u64 = 5 * 60 * 1000;
const AUTH_TTL_MS: u64 = 2 * 60 * 1000;
const TOKEN_BYTES: usize = 32;

#[derive(Deserialize, Serialize)]
struct CeremonyMeta {
    user_id: String,
    session_id: String,
    browser_client_id: String,
    operation: String,
}

#[derive(Deserialize, Serialize)]
struct CeremonyState {
    challenge: Vec<u8>,
    rp_id: String,
    origin: String,
}

#[derive(Deserialize, Serialize)]
struct StoredAuthorization {
    user_id: String,
    browser_client_id: String,
    operation: String,
    issued_at_ms: u64,
    expires_at_ms: u64,
}

pub fn begin(
    session_cookie: &str,
    browser_client_id: &str,
    operation: &str,
    relying_party: RelyingParty,
) -> Result<Challenge, String> {
    let session = session::find(session_cookie)?
        .ok_or_else(|| "active browser session required".to_owned())?;
    validate::id(browser_client_id, "browser client id")?;
    validate::id(operation, "administrator operation")?;
    bounded_text(&relying_party.id, "relying party id", 255)?;
    bounded_text(&relying_party.origin, "relying party origin", 512)?;
    let passkeys = credential::all(Some(&session.principal.user_id))?;
    if passkeys.is_empty() {
        return Err("no passkey is registered".into());
    }
    let challenge = random(32)?;
    let id = random_id()?;
    let now = time::now_ms();
    let metadata = serde_json::to_vec(&CeremonyMeta {
        user_id: session.principal.user_id,
        session_id: session.id,
        browser_client_id: browser_client_id.into(),
        operation: operation.into(),
    })
    .map_err(display)?;
    let state = serde_json::to_vec(&CeremonyState {
        challenge: challenge.clone(),
        rp_id: relying_party.id.clone(),
        origin: relying_party.origin.clone(),
    })
    .map_err(display)?;
    crate::ceremony::put(crate::ohrats::rc_identity::types::Ceremony {
        id: id.clone(),
        kind: CEREMONY_KIND.into(),
        user_id: None,
        metadata,
        state,
        expires_at_ms: now.saturating_add(CEREMONY_TTL_MS),
    })?;
    Ok(Challenge {
        id,
        challenge,
        relying_party,
        credential_ids: passkeys
            .into_iter()
            .map(|value| value.credential.id)
            .collect(),
    })
}

pub fn issue(
    session_cookie: &str,
    browser_client_id: &str,
    challenge_id: &str,
    mut authentication: AuthenticationRequest,
) -> Result<Vec<u8>, String> {
    let session = session::find(session_cookie)?
        .ok_or_else(|| "active browser session required".to_owned())?;
    validate::id(browser_client_id, "browser client id")?;
    validate::id(challenge_id, "administrator ceremony id")?;
    let ceremony = crate::ceremony::take(challenge_id, CEREMONY_KIND)?
        .ok_or_else(|| "administrator ceremony expired".to_owned())?;
    let meta: CeremonyMeta = serde_json::from_slice(&ceremony.metadata).map_err(display)?;
    let state: CeremonyState = serde_json::from_slice(&ceremony.state).map_err(display)?;
    if meta.user_id != session.principal.user_id
        || meta.session_id != session.id
        || meta.browser_client_id != browser_client_id
        || authentication.challenge != state.challenge
        || authentication.relying_party.id != state.rp_id
        || authentication.relying_party.origin != state.origin
    {
        return Err("administrator ceremony binding failed".into());
    }
    let passkey = credential::get_by_credential_id(&authentication.credential_id)?
        .ok_or_else(|| "unknown passkey".to_owned())?;
    if passkey.user_id != session.principal.user_id
        || authentication.expected_user_handle != session.principal.user_id.as_bytes()
    {
        return Err("passkey does not belong to browser user".into());
    }
    let algorithm = passkey.credential.algorithm.clone();
    authentication.credential = passkey.credential.clone();
    let verified = verifier::verify_authentication(&algorithm, &authentication)?;
    if !verified.user_verified {
        return Err("fresh passkey verification required".into());
    }
    let issued_at_ms = time::now_ms();
    credential::update(&passkey.id, verified.credential, issued_at_ms)?;
    let mut token = [0; TOKEN_BYTES];
    getrandom::fill(&mut token).map_err(display)?;
    let stored = StoredAuthorization {
        user_id: meta.user_id,
        browser_client_id: meta.browser_client_id,
        operation: meta.operation,
        issued_at_ms,
        expires_at_ms: issued_at_ms.saturating_add(AUTH_TTL_MS),
    };
    storage::insert(
        AUTH_BUCKET,
        &key(&token),
        serde_json::to_vec(&stored).map_err(display)?,
    )?;
    Ok(token.to_vec())
}

pub fn consume(token: &[u8], operation: &str) -> Result<Claim, String> {
    validate::id(operation, "administrator operation")?;
    if token.len() != TOKEN_BYTES {
        return Err("invalid administrator authorization".into());
    }
    let Some(value) = storage::take(AUTH_BUCKET, &key(token))? else {
        return Err("administrator authorization already consumed or withdrawn".into());
    };
    let value: StoredAuthorization = serde_json::from_slice(&value).map_err(display)?;
    let consumed_at_ms = time::now_ms();
    if value.expires_at_ms <= consumed_at_ms || value.operation != operation {
        return Err("administrator authorization rejected".into());
    }
    Ok(Claim {
        user_id: value.user_id,
        browser_client_id: value.browser_client_id,
        issued_at_ms: value.issued_at_ms,
        consumed_at_ms,
        expires_at_ms: value.expires_at_ms,
    })
}

pub fn withdraw() {
    let Ok(entries) = storage::scan_all(AUTH_BUCKET) else {
        return;
    };
    let _ = storage::remove_many(
        entries
            .into_iter()
            .map(|entry| storage::Delete {
                bucket: AUTH_BUCKET.into(),
                key: entry.key,
            })
            .collect(),
    );
}

fn key(token: &[u8]) -> Vec<u8> {
    Sha256::digest(token).to_vec()
}

fn random(size: usize) -> Result<Vec<u8>, String> {
    let mut bytes = vec![0; size];
    getrandom::fill(&mut bytes).map_err(display)?;
    Ok(bytes)
}

fn random_id() -> Result<String, String> {
    Ok(hex(&random(18)?))
}

fn hex(bytes: &[u8]) -> String {
    bytes.iter().map(|value| format!("{value:02x}")).collect()
}

fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}

fn bounded_text(value: &str, label: &str, max: usize) -> Result<(), String> {
    if value.is_empty() || value.len() > max || value.chars().any(char::is_control) {
        Err(format!("invalid {label}"))
    } else {
        Ok(())
    }
}
