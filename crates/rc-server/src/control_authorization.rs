use crate::{
    AppState, CONTROL_DEFAULT_LIFETIME, UserIdentity, auth_lifetime, now_ms, passkey_public_key,
    user_passkeys,
};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_crypto::{control_grant_challenge, verify_ed25519, verify_webauthn_assertion};
use rc_protocol::{ControlGrant, ControlProof};
use rusqlite::OptionalExtension;
use serde::{Deserialize, Serialize};
use uuid::Uuid;
use webauthn_rs::prelude::{Base64UrlSafeData, PublicKeyCredential, RequestChallengeResponse};

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ControlAuthorizationStart {
    pub authorization_id: String,
    pub grant: String,
    pub options: RequestChallengeResponse,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ControlAuthorizationStatus {
    pub authorized: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub expires_at: Option<i64>,
}

#[derive(Debug, Serialize, Deserialize)]
struct AuthorizationMeta {
    client_id: String,
    signing_public_key: String,
    grant: String,
    client_kind: String,
}

pub fn start_control_authorization(
    state: &AppState,
    user: &UserIdentity,
    client_id: &str,
    signing_public_key: &str,
    lifetime: Option<&str>,
    client_kind: &str,
) -> anyhow::Result<ControlAuthorizationStart> {
    validate_client(client_id, signing_public_key, client_kind)?;
    let issued_at = now_ms();
    let lifetime = auth_lifetime(lifetime, CONTROL_DEFAULT_LIFETIME, true, issued_at)
        .map_err(anyhow::Error::msg)?;
    let grant = serde_json::to_string(&ControlGrant {
        v: 1,
        client_id: client_id.to_owned(),
        user_id: user.id.clone(),
        signing_public_key: signing_public_key.to_owned(),
        issued_at,
        expires_at: lifetime.expires_at,
    })?;
    let challenge = control_grant_challenge(&grant);
    let passkeys = user_passkeys(state, &user.id)?;
    if passkeys.is_empty() {
        anyhow::bail!("no passkeys registered");
    }
    let (mut options, _) = state.webauthn.start_passkey_authentication(&passkeys)?;
    options.public_key.challenge = Base64UrlSafeData::from(URL_SAFE_NO_PAD.decode(&challenge)?);
    let authorization_id = Uuid::new_v4().to_string();
    let meta = AuthorizationMeta {
        client_id: client_id.to_owned(),
        signing_public_key: signing_public_key.to_owned(),
        grant: grant.clone(),
        client_kind: client_kind.to_owned(),
    };
    let meta_json = serde_json::to_string(&meta)?;
    state.db.with_connection(|db| {
        db.execute(
            "INSERT INTO ceremonies(id,kind,user_id,meta_json,state_json,expires_at) VALUES(?,'control-authorize',? ,?,'{}',?)",
            rusqlite::params![authorization_id, user.id, meta_json, issued_at + 5 * 60_000],
        )?;
        Ok(())
    })?;
    Ok(ControlAuthorizationStart {
        authorization_id,
        grant,
        options,
    })
}

pub fn finish_control_authorization(
    state: &AppState,
    user: &UserIdentity,
    authorization_id: &str,
    response: serde_json::Value,
) -> anyhow::Result<(String, i64)> {
    let meta = state.db.with_connection_mut(|db| {
        let tx = db.transaction()?;
        let meta = tx
            .query_row(
                "SELECT meta_json FROM ceremonies WHERE id=? AND kind='control-authorize' AND user_id=? AND expires_at>?",
                rusqlite::params![authorization_id, user.id, now_ms()],
                |row| row.get::<_, String>(0),
            )
            .optional()?;
        tx.execute("DELETE FROM ceremonies WHERE id=?", [authorization_id])?;
        tx.commit()?;
        Ok(meta)
    })?;
    let meta: AuthorizationMeta = serde_json::from_str(
        &meta.ok_or_else(|| anyhow::anyhow!("control authorization expired"))?,
    )?;
    let assertion: PublicKeyCredential = serde_json::from_value(response.clone())?;
    let credential_id = URL_SAFE_NO_PAD.encode(assertion.raw_id.as_ref());
    let public_key = passkey_public_key(state, &user.id, &credential_id)?
        .ok_or_else(|| anyhow::anyhow!("unknown passkey"))?;
    let origin = state.config.public_url.trim_end_matches('/');
    let rp_id = url::Url::parse(origin)?
        .host_str()
        .ok_or_else(|| anyhow::anyhow!("invalid RC origin"))?
        .to_owned();
    verify_webauthn_assertion(
        &serde_json::to_string(&response)?,
        &credential_id,
        &public_key,
        &control_grant_challenge(&meta.grant),
        origin,
        &rp_id,
    )?;
    let grant: ControlGrant = serde_json::from_str(&meta.grant)?;
    let assertion_json = serde_json::to_string(&response)?;
    state.db.with_connection_mut(|db| {
        let tx = db.transaction()?;
        let changed = tx.execute(
            "INSERT INTO clients(id,user_id,kind,name,public_key,scopes,credential_id,grant,assertion,created_at,expires_at,last_used) VALUES(?,?,?,?,?,'[]',?,?,?,?,?,NULL) ON CONFLICT(id) DO UPDATE SET kind=excluded.kind,name=excluded.name,public_key=excluded.public_key,credential_id=excluded.credential_id,grant=excluded.grant,assertion=excluded.assertion,created_at=excluded.created_at,expires_at=excluded.expires_at,last_used=NULL WHERE clients.user_id=excluded.user_id AND clients.kind IN ('browser','cli')",
            rusqlite::params![
                meta.client_id,
                user.id,
                meta.client_kind,
                if meta.client_kind == "cli" { "RC CLI" } else { "Browser" },
                meta.signing_public_key,
                credential_id,
                meta.grant,
                assertion_json,
                grant.issued_at,
                grant.expires_at,
            ],
        )?;
        if changed != 1 {
            return Err(rusqlite::Error::InvalidQuery);
        }
        tx.commit()
    })?;
    Ok((grant.client_id, grant.expires_at))
}

pub fn control_client_status(
    state: &AppState,
    user_id: &str,
    client_id: &str,
) -> anyhow::Result<ControlAuthorizationStatus> {
    let expires_at = state.db.with_connection(|db| {
        db.query_row(
            "SELECT expires_at FROM clients WHERE id=? AND user_id=? AND kind IN ('browser','cli') AND (expires_at=0 OR expires_at>?)",
            rusqlite::params![client_id, user_id, now_ms()],
            |row| row.get::<_, i64>(0),
        )
        .optional()
    })?;
    Ok(ControlAuthorizationStatus {
        authorized: expires_at.is_some(),
        expires_at,
    })
}

pub fn control_proof(
    state: &AppState,
    user_id: &str,
    client_id: &str,
) -> anyhow::Result<Option<ControlProof>> {
    state.db.with_connection(|db| {
        db.query_row(
            "SELECT grant,credential_id,assertion FROM clients WHERE id=? AND user_id=? AND kind IN ('browser','cli') AND (expires_at=0 OR expires_at>?)",
            rusqlite::params![client_id, user_id, now_ms()],
            |row| Ok(ControlProof {
                grant: row.get(0)?,
                credential_id: row.get(1)?,
                assertion: row.get(2)?,
            }),
        )
        .optional()
    }).map_err(Into::into)
}

pub fn fresh_control_proof(
    state: &AppState,
    user_id: &str,
    client_id: &str,
    max_age_ms: i64,
) -> anyhow::Result<Option<ControlProof>> {
    state.db.with_connection(|db| {
        db.query_row(
            "SELECT grant,credential_id,assertion FROM clients WHERE id=? AND user_id=? AND kind IN ('browser','cli') AND created_at>=? AND (expires_at=0 OR expires_at>?)",
            rusqlite::params![client_id, user_id, now_ms() - max_age_ms, now_ms()],
            |row| Ok(ControlProof {
                grant: row.get(0)?, credential_id: row.get(1)?, assertion: row.get(2)?,
            }),
        ).optional()
    }).map_err(Into::into)
}

pub fn verify_control_client_signature(
    state: &AppState,
    user_id: &str,
    client_id: &str,
    payload: &str,
    signature: &str,
) -> anyhow::Result<bool> {
    let public_key = state.db.with_connection(|db| {
        db.query_row(
            "SELECT public_key FROM clients WHERE id=? AND user_id=? AND kind IN ('browser','cli') AND (expires_at=0 OR expires_at>?)",
            rusqlite::params![client_id, user_id, now_ms()],
            |row| row.get::<_, String>(0),
        ).optional()
    })?;
    Ok(public_key.is_some_and(|key| verify_ed25519(&key, payload.as_bytes(), signature).is_ok()))
}

fn validate_client(client_id: &str, public_key: &str, kind: &str) -> anyhow::Result<()> {
    if client_id.len() < 16
        || client_id.len() > 100
        || !client_id
            .bytes()
            .all(|value| value.is_ascii_alphanumeric() || matches!(value, b'-' | b'_'))
        || !matches!(kind, "browser" | "cli")
    {
        anyhow::bail!("invalid control client key");
    }
    if URL_SAFE_NO_PAD
        .decode(public_key)
        .map(|bytes| bytes.len())
        .unwrap_or(0)
        != 32
    {
        anyhow::bail!("invalid control client key");
    }
    Ok(())
}
