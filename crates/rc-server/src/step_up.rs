use crate::{AppState, UserIdentity, hash, now_ms, opaque, passkey_public_key, user_passkeys};
use axum::http::HeaderMap;
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_crypto::verify_webauthn_assertion;
use rusqlite::OptionalExtension;
use serde::{Deserialize, Serialize};
use uuid::Uuid;
use webauthn_rs::prelude::{PublicKeyCredential, RequestChallengeResponse};

const AUTH_TTL_MS: i64 = 5 * 60 * 1000;
const TOKEN_TTL_MS: i64 = 2 * 60 * 1000;
const RECENT_SESSION_TTL_MS: i64 = 5 * 60 * 1000;

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StepUpStart {
    pub authorization_id: String,
    pub options: RequestChallengeResponse,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StepUpResult {
    pub token: String,
    pub expires_at: i64,
}

#[derive(Serialize, Deserialize)]
struct StepMeta {
    challenge: String,
}

pub fn start_step_up(state: &AppState, user: &UserIdentity) -> anyhow::Result<StepUpStart> {
    let passkeys = user_passkeys(state, &user.id)?;
    if passkeys.is_empty() {
        anyhow::bail!("no passkeys registered");
    }
    let (options, _) = state.webauthn.start_passkey_authentication(&passkeys)?;
    let challenge = URL_SAFE_NO_PAD.encode(options.public_key.challenge.as_ref());
    let authorization_id = Uuid::new_v4().to_string();
    let meta_json = serde_json::to_string(&StepMeta { challenge })?;
    state.db.with_connection(|db| {
        db.execute(
            "INSERT INTO ceremonies(id,kind,user_id,meta_json,state_json,expires_at) VALUES(?,'step-up',?,?,'{}',?)",
            rusqlite::params![
                authorization_id,
                user.id,
                meta_json,
                now_ms() + AUTH_TTL_MS
            ],
        )?;
        Ok(())
    })?;
    Ok(StepUpStart {
        authorization_id,
        options,
    })
}

pub fn finish_step_up(
    state: &AppState,
    user: &UserIdentity,
    authorization_id: &str,
    response: serde_json::Value,
) -> anyhow::Result<StepUpResult> {
    let meta = state.db.with_connection_mut(|db| {
        let tx = db.transaction()?;
        let meta = tx
            .query_row(
                "SELECT meta_json FROM ceremonies WHERE id=? AND kind='step-up' AND user_id=? AND expires_at>?",
                rusqlite::params![authorization_id, user.id, now_ms()],
                |row| row.get::<_, String>(0),
            )
            .optional()?;
        tx.execute("DELETE FROM ceremonies WHERE id=?", [authorization_id])?;
        tx.commit()?;
        Ok(meta)
    })?;
    let meta: StepMeta =
        serde_json::from_str(&meta.ok_or_else(|| anyhow::anyhow!("passkey step-up expired"))?)?;
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
        &meta.challenge,
        origin,
        &rp_id,
    )?;
    let token = opaque(24);
    let expires_at = now_ms() + TOKEN_TTL_MS;
    state.db.with_connection(|db| {
        db.execute(
            "INSERT INTO ceremonies(id,kind,user_id,meta_json,state_json,expires_at) VALUES(?,'step-token',?,'{}','{}',?)",
            rusqlite::params![hash(&token), user.id, expires_at],
        )?;
        db.execute(
            "UPDATE passkeys SET last_used=? WHERE user_id=?",
            rusqlite::params![now_ms(), user.id],
        )?;
        Ok(())
    })?;
    Ok(StepUpResult { token, expires_at })
}

pub fn consume_step_up(state: &AppState, headers: &HeaderMap, user_id: &str) -> anyhow::Result<()> {
    let token = headers
        .get("x-rc-step-up")
        .and_then(|value| value.to_str().ok())
        .unwrap_or_default();
    if token.is_empty() {
        anyhow::bail!("fresh passkey verification required");
    }
    let changed = state.db.with_connection(|db| {
        db.execute(
            "DELETE FROM ceremonies WHERE id=? AND kind='step-token' AND user_id=? AND expires_at>?",
            rusqlite::params![hash(token), user_id, now_ms()],
        )
    })?;
    if changed != 1 {
        anyhow::bail!("fresh passkey verification required");
    }
    Ok(())
}

pub fn recent_browser_session(
    state: &AppState,
    headers: &HeaderMap,
    user_id: &str,
) -> anyhow::Result<bool> {
    let token = cookie(headers, "rc_session").unwrap_or_default();
    if token.is_empty() {
        return Ok(false);
    }
    Ok(state.db.with_connection(|db| {
        db.query_row(
            "SELECT 1 FROM sessions WHERE token_hash=? AND user_id=? AND kind='browser' AND created_at>? AND (expires_at=0 OR expires_at>?)",
            rusqlite::params![hash(&token), user_id, now_ms() - RECENT_SESSION_TTL_MS, now_ms()],
            |_| Ok(()),
        ).optional().map(|value| value.is_some())
    })?)
}

fn cookie(headers: &HeaderMap, name: &str) -> Option<String> {
    headers
        .get("cookie")?
        .to_str()
        .ok()?
        .split(';')
        .filter_map(|part| part.trim().split_once('='))
        .find(|(key, _)| *key == name)
        .map(|(_, value)| value.to_owned())
}
