use super::{McpGrantRecord, mcp_resource};
use crate::{AppState, hash, now_ms, opaque};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_protocol::McpGrantPayload;
use rusqlite::OptionalExtension;
use sha2::{Digest, Sha256};

pub fn exchange_token(state: &AppState, form: &str) -> anyhow::Result<serde_json::Value> {
    let params = url::form_urlencoded::parse(form.as_bytes())
        .into_owned()
        .collect::<std::collections::HashMap<_, _>>();
    if params.get("resource").map(String::as_str) != Some(mcp_resource(state).as_str()) {
        anyhow::bail!("invalid resource");
    }
    let client = params.get("client_id").cloned().unwrap_or_default();
    let grant = match params.get("grant_type").map(String::as_str) {
        Some("authorization_code") => {
            let verifier = params.get("code_verifier").cloned().unwrap_or_default();
            if !(43..=128).contains(&verifier.len())
                || !verifier
                    .chars()
                    .all(|c| c.is_ascii_alphanumeric() || "-._~".contains(c))
            {
                anyhow::bail!("invalid PKCE verifier");
            }
            take_code(
                state,
                params.get("code").map(String::as_str).unwrap_or_default(),
                &client,
                params
                    .get("redirect_uri")
                    .map(String::as_str)
                    .unwrap_or_default(),
                &verifier,
            )?
        }
        Some("refresh_token") => take_refresh(
            state,
            params
                .get("refresh_token")
                .map(String::as_str)
                .unwrap_or_default(),
            &client,
        )?,
        _ => anyhow::bail!("unsupported grant_type"),
    };
    issue_tokens(state, &grant)
}

pub fn access_grant(state: &AppState, token: &str) -> anyhow::Result<Option<McpGrantRecord>> {
    let value = state.db.with_connection(|db| {
        db.query_row(
            "SELECT g.id,g.user_id,g.client_id,g.name,g.grant,g.grant_signature,g.client_control_id,g.credential_id,g.control_grant,g.control_assertion,g.expires_at FROM oauth_tokens t JOIN mcp_grants g ON g.id=t.grant_id WHERE t.token_hash=? AND t.kind='access' AND t.expires_at>? AND g.revoked_at IS NULL AND (g.expires_at=0 OR g.expires_at>?)",
            rusqlite::params![hash(token),now_ms(),now_ms()],
            grant_row,
        ).optional()
    })?;
    if let Some(grant) = &value
        && let Err(error) = state.db.with_connection(|db| {
            db.execute(
                "UPDATE mcp_grants SET last_used=? WHERE id=?",
                rusqlite::params![now_ms(), grant.id],
            )
        })
    {
        tracing::warn!(grant_id = %grant.id, %error, "failed to update MCP grant usage");
    }
    Ok(value)
}

pub fn revoke_mcp_grant(state: &AppState, user_id: &str, id: &str) -> anyhow::Result<bool> {
    let changed = state.db.with_connection_mut(|db| {
        let tx = db.transaction()?;
        let changed = tx.execute(
            "UPDATE mcp_grants SET revoked_at=COALESCE(revoked_at,?) WHERE id=? AND user_id=?",
            rusqlite::params![now_ms(), id, user_id],
        )?;
        tx.execute("DELETE FROM oauth_tokens WHERE grant_id=?", [id])?;
        tx.commit()?;
        Ok(changed)
    })?;
    Ok(changed > 0)
}

fn issue_tokens(state: &AppState, grant: &McpGrantRecord) -> anyhow::Result<serde_json::Value> {
    let payload: McpGrantPayload = serde_json::from_str(&grant.grant)?;
    let now = now_ms();
    let ttl = (state.config.mcp_access_ttl_minutes as i64) * 60_000;
    let access_exp = if grant.expires_at == 0 {
        now + ttl
    } else {
        (now + ttl).min(grant.expires_at)
    };
    let access = format!("mcp_access_{}", opaque(24));
    let refresh = format!("mcp_refresh_{}", opaque(24));
    state.db.with_connection_mut(|db| {
        let tx = db.transaction()?;
        tx.execute(
            "INSERT INTO oauth_tokens(token_hash,grant_id,kind,expires_at) VALUES(?,?,'access',?)",
            rusqlite::params![hash(&access), grant.id, access_exp],
        )?;
        tx.execute(
            "INSERT INTO oauth_tokens(token_hash,grant_id,kind,expires_at) VALUES(?,?,'refresh',?)",
            rusqlite::params![hash(&refresh), grant.id, grant.expires_at],
        )?;
        tx.execute(
            "UPDATE mcp_grants SET last_used=? WHERE id=?",
            rusqlite::params![now, grant.id],
        )?;
        tx.commit()
    })?;
    Ok(serde_json::json!({
        "access_token":access,"token_type":"Bearer","expires_in":((access_exp-now)/1000).max(1),
        "refresh_token":refresh,"scope":payload.scopes.join(" ")
    }))
}

fn take_code(
    state: &AppState,
    code: &str,
    client: &str,
    redirect: &str,
    verifier: &str,
) -> anyhow::Result<McpGrantRecord> {
    state.db.with_connection_mut(|db| {
        let tx = db.transaction()?;
        let value = tx.query_row(
            "SELECT g.id,g.user_id,g.client_id,g.name,g.grant,g.grant_signature,g.client_control_id,g.credential_id,g.control_grant,g.control_assertion,g.expires_at,c.redirect_uri,c.code_challenge FROM oauth_codes c JOIN mcp_grants g ON g.id=c.grant_id WHERE c.code_hash=? AND c.expires_at>? AND g.revoked_at IS NULL AND (g.expires_at=0 OR g.expires_at>?)",
            rusqlite::params![hash(code),now_ms(),now_ms()],
            |row| Ok((grant_row(row)?, row.get::<_,String>(11)?, row.get::<_,String>(12)?)),
        ).optional()?;
        let Some((grant, expected_redirect, challenge)) = value else {
            return Err(rusqlite::Error::QueryReturnedNoRows);
        };
        if grant.client_id != client || expected_redirect != redirect || challenge != pkce(verifier) {
            return Err(rusqlite::Error::InvalidQuery);
        }
        if tx.execute("DELETE FROM oauth_codes WHERE code_hash=?", [hash(code)])? != 1 {
            return Err(rusqlite::Error::InvalidQuery);
        }
        tx.commit()?;
        Ok(grant)
    }).map_err(|_| anyhow::anyhow!("invalid authorization code"))
}

fn take_refresh(state: &AppState, token: &str, client: &str) -> anyhow::Result<McpGrantRecord> {
    state.db.with_connection_mut(|db| {
        let tx = db.transaction()?;
        let grant = tx.query_row(
            "SELECT g.id,g.user_id,g.client_id,g.name,g.grant,g.grant_signature,g.client_control_id,g.credential_id,g.control_grant,g.control_assertion,g.expires_at FROM oauth_tokens t JOIN mcp_grants g ON g.id=t.grant_id WHERE t.token_hash=? AND t.kind='refresh' AND (t.expires_at=0 OR t.expires_at>?) AND g.revoked_at IS NULL AND (g.expires_at=0 OR g.expires_at>?)",
            rusqlite::params![hash(token),now_ms(),now_ms()],
            grant_row,
        ).optional()?.ok_or(rusqlite::Error::QueryReturnedNoRows)?;
        if grant.client_id != client { return Err(rusqlite::Error::InvalidQuery); }
        if tx.execute("DELETE FROM oauth_tokens WHERE token_hash=?", [hash(token)])? != 1 {
            return Err(rusqlite::Error::InvalidQuery);
        }
        tx.commit()?;
        Ok(grant)
    }).map_err(|_| anyhow::anyhow!("invalid refresh token"))
}

fn pkce(verifier: &str) -> String {
    URL_SAFE_NO_PAD.encode(Sha256::digest(verifier.as_bytes()))
}

fn grant_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<McpGrantRecord> {
    Ok(McpGrantRecord {
        id: row.get(0)?,
        user_id: row.get(1)?,
        client_id: row.get(2)?,
        name: row.get(3)?,
        grant: row.get(4)?,
        grant_signature: row.get(5)?,
        client_control_id: row.get(6)?,
        credential_id: row.get(7)?,
        control_grant: row.get(8)?,
        control_assertion: row.get(9)?,
        expires_at: row.get(10)?,
    })
}
