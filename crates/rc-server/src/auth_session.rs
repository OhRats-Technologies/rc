use crate::{AppState, ClientHttpError, ClientIdentity, now_ms, verify_client_request};
use axum::http::{HeaderMap, Method, StatusCode};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rand::RngCore;
use sha2::{Digest, Sha256};

pub const DELETED_USER_ID: &str = "__deleted_account__";

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize)]
pub struct UserIdentity {
    pub id: String,
    pub name: String,
}

#[derive(Debug, Clone)]
pub struct AuthPrincipal {
    pub user: UserIdentity,
    pub client: Option<ClientIdentity>,
    pub browser: bool,
}

pub fn authenticate(
    state: &AppState,
    headers: &HeaderMap,
    method: &Method,
    request_uri: &str,
    body: &[u8],
) -> Result<Option<AuthPrincipal>, AuthError> {
    if headers.contains_key("x-rc-key-id") {
        let client = verify_client_request(state, headers, method, request_uri, body)?;
        let user = user_by_id(state, &client.user_id)?.ok_or(AuthError::Unauthorized)?;
        return Ok(Some(AuthPrincipal {
            user,
            client: Some(client),
            browser: false,
        }));
    }
    if headers.contains_key("authorization") {
        return Err(AuthError::Unauthorized);
    }
    let Some(token) = cookie(headers, "rc_session") else {
        return Ok(None);
    };
    let user = state
        .db
        .with_connection(|db| {
            use rusqlite::OptionalExtension;
            db.query_row(
                "SELECT u.id,u.name FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.kind='browser' AND (s.expires_at=0 OR s.expires_at>?)",
                rusqlite::params![hash(&token), now_ms()],
                |row| Ok(UserIdentity { id: row.get(0)?, name: row.get(1)? }),
            )
            .optional()
        })
        .map_err(|_| AuthError::Database)?;
    Ok(user.map(|user| AuthPrincipal {
        user,
        client: None,
        browser: true,
    }))
}

pub fn browser_user(
    state: &AppState,
    headers: &HeaderMap,
) -> Result<Option<UserIdentity>, AuthError> {
    let Some(token) = cookie(headers, "rc_session") else {
        return Ok(None);
    };
    state
        .db
        .with_connection(|db| {
            use rusqlite::OptionalExtension;
            db.query_row(
                "SELECT u.id,u.name FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.kind='browser' AND (s.expires_at=0 OR s.expires_at>?)",
                rusqlite::params![hash(&token), now_ms()],
                |row| Ok(UserIdentity { id: row.get(0)?, name: row.get(1)? }),
            )
            .optional()
        })
        .map_err(|_| AuthError::Database)
}

pub fn create_browser_session(
    state: &AppState,
    user_id: &str,
    expires_at: i64,
) -> Result<String, AuthError> {
    let token = opaque(32);
    state
        .db
        .with_connection(|db| {
            db.execute(
                "INSERT INTO sessions(token_hash,user_id,kind,created_at,expires_at) VALUES(?,?,'browser',?,?)",
                rusqlite::params![hash(&token), user_id, now_ms(), expires_at],
            )?;
            Ok(())
        })
        .map_err(|_| AuthError::Database)?;
    Ok(token)
}

pub fn revoke_browser_session(state: &AppState, headers: &HeaderMap) -> Result<(), AuthError> {
    let Some(token) = cookie(headers, "rc_session") else {
        return Ok(());
    };
    state
        .db
        .with_connection(|db| {
            db.execute("DELETE FROM sessions WHERE token_hash=?", [hash(&token)])?;
            Ok(())
        })
        .map_err(|_| AuthError::Database)
}

pub fn session_cookie(state: &AppState, token: &str, max_age: i64) -> String {
    let mut cookie = format!(
        "rc_session={token}; Path=/; HttpOnly; SameSite=Lax; Max-Age={}",
        max_age.max(0)
    );
    if state.config.public_url.starts_with("https://") {
        cookie.push_str("; Secure");
    }
    cookie
}

pub fn clear_session_cookie(state: &AppState) -> String {
    session_cookie(state, "", 0)
}

pub fn user_by_id(state: &AppState, id: &str) -> Result<Option<UserIdentity>, AuthError> {
    state
        .db
        .with_connection(|db| {
            use rusqlite::OptionalExtension;
            db.query_row("SELECT id,name FROM users WHERE id=?", [id], |row| {
                Ok(UserIdentity {
                    id: row.get(0)?,
                    name: row.get(1)?,
                })
            })
            .optional()
        })
        .map_err(|_| AuthError::Database)
}

pub fn active_user_count(state: &AppState) -> Result<i64, AuthError> {
    state
        .db
        .with_connection(|db| {
            db.query_row(
                "SELECT count(*) FROM users WHERE id<>?",
                [DELETED_USER_ID],
                |row| row.get(0),
            )
        })
        .map_err(|_| AuthError::Database)
}

pub fn opaque(size: usize) -> String {
    let mut bytes = vec![0_u8; size];
    rand::rngs::OsRng.fill_bytes(&mut bytes);
    URL_SAFE_NO_PAD.encode(bytes)
}

pub fn hash(value: &str) -> String {
    Sha256::digest(value.as_bytes())
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect()
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

#[derive(Debug, thiserror::Error)]
pub enum AuthError {
    #[error("authentication required")]
    Unauthorized,
    #[error("database error")]
    Database,
    #[error("client authentication failed")]
    Client,
}

impl From<ClientHttpError> for AuthError {
    fn from(_: ClientHttpError) -> Self {
        Self::Client
    }
}

impl AuthError {
    pub fn status(&self) -> StatusCode {
        match self {
            Self::Unauthorized | Self::Client => StatusCode::UNAUTHORIZED,
            Self::Database => StatusCode::INTERNAL_SERVER_ERROR,
        }
    }
}
