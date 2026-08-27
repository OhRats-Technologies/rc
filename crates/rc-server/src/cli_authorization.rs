use crate::{AppState, CLI_DEFAULT_LIFETIME, UserIdentity, auth_lifetime, hash, now_ms, opaque};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rusqlite::OptionalExtension;
use serde::Serialize;
use uuid::Uuid;

const REQUEST_TTL_MS: i64 = 10 * 60 * 1000;

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct CliAuthorizationStart {
    pub request_id: String,
    pub device_code: String,
    pub expires_at: i64,
    pub interval: u64,
    pub verification_url: String,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase", untagged)]
pub enum CliPollResult {
    Pending { pending: bool },
    Approved { pending: bool, user: UserIdentity },
}

pub fn start_cli_authorization(
    state: &AppState,
    client_id: &str,
    public_key: &str,
    lifetime: Option<&str>,
) -> anyhow::Result<CliAuthorizationStart> {
    if client_id.is_empty()
        || client_id.len() > 100
        || URL_SAFE_NO_PAD
            .decode(public_key)
            .map(|bytes| bytes.len())
            .unwrap_or(0)
            != 32
    {
        anyhow::bail!("invalid CLI control key");
    }
    auth_lifetime(lifetime, CLI_DEFAULT_LIFETIME, true, now_ms()).map_err(anyhow::Error::msg)?;
    let request_id = Uuid::new_v4().to_string();
    let device_code = format!("cli_device_{}", opaque(24));
    let user_code = format!("cli_user_{}", opaque(12));
    let expires_at = now_ms() + REQUEST_TTL_MS;
    state.db.with_connection(|db| {
        db.execute(
            "DELETE FROM cli_authorizations WHERE expires_at<=? OR exchanged_at IS NOT NULL",
            [now_ms()],
        )?;
        db.execute(
            "INSERT INTO cli_authorizations(id,device_code_hash,user_code_hash,client_id,public_key,lifetime,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?)",
            rusqlite::params![
                request_id,
                hash(&device_code),
                hash(&user_code),
                client_id,
                public_key,
                lifetime.unwrap_or(CLI_DEFAULT_LIFETIME),
                now_ms(),
                expires_at,
            ],
        )?;
        Ok(())
    })?;
    Ok(CliAuthorizationStart {
        request_id,
        device_code,
        expires_at,
        interval: 2,
        verification_url: format!(
            "{}/cli/login?code={}",
            state.config.public_url.trim_end_matches('/'),
            user_code
        ),
    })
}

pub fn approve_cli_authorization(
    state: &AppState,
    user: &UserIdentity,
    code: &str,
) -> anyhow::Result<()> {
    let code_hash = hash(code.trim());
    state.db.with_connection_mut(|db| {
        let tx = db.transaction()?;
        let row = tx
            .query_row(
                "SELECT id,client_id,public_key,lifetime,approved_at FROM cli_authorizations WHERE user_code_hash=? AND expires_at>? AND exchanged_at IS NULL",
                rusqlite::params![code_hash, now_ms()],
                |row| Ok((row.get::<_, String>(0)?, row.get::<_, String>(1)?, row.get::<_, String>(2)?, row.get::<_, String>(3)?, row.get::<_, Option<i64>>(4)?)),
            )
            .optional()?;
        let Some((id, client_id, public_key, lifetime, approved_at)) = row else {
            return Err(rusqlite::Error::InvalidQuery);
        };
        if approved_at.is_some() {
            return Err(rusqlite::Error::InvalidQuery);
        }
        let client: Option<(String, String)> = tx
            .query_row(
                "SELECT user_id,public_key FROM clients WHERE id=? AND kind='browser' AND (expires_at=0 OR expires_at>?)",
                rusqlite::params![client_id, now_ms()],
                |row| Ok((row.get(0)?, row.get(1)?)),
            )
            .optional()?;
        if client.as_ref().is_none_or(|(client_user, key)| client_user != &user.id || key != &public_key) {
            return Err(rusqlite::Error::InvalidQuery);
        }
        let expiration = auth_lifetime(Some(&lifetime), CLI_DEFAULT_LIFETIME, true, now_ms())
            .map_err(|_| rusqlite::Error::InvalidQuery)?
            .expires_at;
        tx.execute(
            "UPDATE clients SET kind='cli',name='RC CLI',expires_at=? WHERE id=? AND user_id=?",
            rusqlite::params![expiration, client_id, user.id],
        )?;
        tx.execute(
            "UPDATE cli_authorizations SET user_id=?,approved_at=? WHERE id=?",
            rusqlite::params![user.id, now_ms(), id],
        )?;
        tx.commit()?;
        Ok(())
    })
    .map_err(|_| anyhow::anyhow!("CLI authorization expired"))
}

pub fn poll_cli_authorization(
    state: &AppState,
    request_id: &str,
    device_code: &str,
) -> anyhow::Result<CliPollResult> {
    state.db.with_connection_mut(|db| {
        let tx = db.transaction()?;
        let row = tx
            .query_row(
                "SELECT user_id,approved_at,exchanged_at FROM cli_authorizations WHERE id=? AND device_code_hash=? AND expires_at>?",
                rusqlite::params![request_id, hash(device_code), now_ms()],
                |row| Ok((row.get::<_, Option<String>>(0)?, row.get::<_, Option<i64>>(1)?, row.get::<_, Option<i64>>(2)?)),
            )
            .optional()?;
        let Some((user_id, approved_at, exchanged_at)) = row else {
            return Err(rusqlite::Error::InvalidQuery);
        };
        if exchanged_at.is_some() {
            return Err(rusqlite::Error::InvalidQuery);
        }
        let Some(user_id) = user_id.filter(|_| approved_at.is_some()) else {
            tx.commit()?;
            return Ok(CliPollResult::Pending { pending: true });
        };
        let user = tx.query_row("SELECT id,name FROM users WHERE id=?", [&user_id], |row| {
            Ok(UserIdentity { id: row.get(0)?, name: row.get(1)? })
        })?;
        tx.execute(
            "UPDATE cli_authorizations SET exchanged_at=? WHERE id=?",
            rusqlite::params![now_ms(), request_id],
        )?;
        tx.commit()?;
        Ok(CliPollResult::Approved { pending: false, user })
    }).map_err(|_| anyhow::anyhow!("CLI authorization expired"))
}

pub fn cli_authorization_preview(
    state: &AppState,
    code: &str,
) -> anyhow::Result<Option<(String, String, String)>> {
    Ok(state.db.with_connection(|db| {
        db.query_row(
            "SELECT client_id,public_key,lifetime FROM cli_authorizations WHERE user_code_hash=? AND expires_at>? AND exchanged_at IS NULL",
            rusqlite::params![hash(code.trim()), now_ms()],
            |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?)),
        ).optional()
    })?)
}

pub fn revoke_cli_client(state: &AppState, client_id: &str, user_id: &str) -> anyhow::Result<bool> {
    Ok(state.db.with_connection(|db| {
        db.execute(
            "DELETE FROM clients WHERE id=? AND user_id=? AND kind='cli'",
            rusqlite::params![client_id, user_id],
        )
        .map(|changes| changes == 1)
    })?)
}
