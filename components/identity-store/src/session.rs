use crate::{
    ohrats::rc_session::types::{IssuedSession, Principal, Session},
    storage, time, user,
};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

const BUCKET: &str = "browser-sessions";
const COOKIE: &str = "rc_session";
const TOKEN_BYTES: usize = 32;
const SESSION_ID_BYTES: usize = 16;
const MAX_COOKIE_HEADER_BYTES: usize = 8192;

#[derive(Serialize, Deserialize)]
struct StoredSession {
    id: String,
    user_id: String,
    created_at_ms: u64,
    expires_at_ms: u64,
}

pub fn issue(user_id: String, expires_at_ms: u64) -> Result<IssuedSession, String> {
    let principal = user::get(&user_id)?.ok_or_else(|| "user not found".to_owned())?;
    let created_at_ms = time::now_ms();
    if expires_at_ms != 0 && expires_at_ms <= created_at_ms {
        return Err("browser session expiration must be in the future".into());
    }
    let token = random_token(TOKEN_BYTES)?;
    let stored = StoredSession {
        id: random_token(SESSION_ID_BYTES)?,
        user_id,
        created_at_ms,
        expires_at_ms,
    };
    storage::insert(BUCKET, &token_key(&token), encode(&stored)?)?;
    Ok(IssuedSession {
        token,
        session: wire(stored, principal.display_name),
    })
}

pub fn find(cookie_header: &str) -> Result<Option<Session>, String> {
    let Some(token) = cookie_token(cookie_header) else {
        return Ok(None);
    };
    let key = token_key(&token);
    let Some(value) = storage::get(BUCKET, &key)? else {
        return Ok(None);
    };
    let stored: StoredSession = serde_json::from_slice(&value).map_err(display)?;
    if expired(&stored) {
        storage::remove(BUCKET, &key)?;
        return Ok(None);
    }
    let Some(principal) = user::get(&stored.user_id)? else {
        storage::remove(BUCKET, &key)?;
        return Ok(None);
    };
    Ok(Some(wire(stored, principal.display_name)))
}

pub fn revoke(cookie_header: &str) -> Result<bool, String> {
    let mut revoked = false;
    for token in cookie_tokens(cookie_header) {
        revoked |= storage::remove(BUCKET, &token_key(&token))?;
    }
    Ok(revoked)
}

fn wire(value: StoredSession, display_name: String) -> Session {
    Session {
        id: value.id,
        principal: Principal {
            user_id: value.user_id,
            display_name,
        },
        created_at_ms: value.created_at_ms,
        expires_at_ms: value.expires_at_ms,
    }
}

fn expired(value: &StoredSession) -> bool {
    value.expires_at_ms != 0 && value.expires_at_ms <= time::now_ms()
}

fn cookie_token(header: &str) -> Option<String> {
    if header.len() > MAX_COOKIE_HEADER_BYTES {
        return None;
    }
    let mut found = None;
    for part in header.split(';') {
        let Some((name, value)) = part.trim().split_once('=') else {
            continue;
        };
        if name != COOKIE {
            continue;
        }
        if found.is_some() || !valid_token(value) {
            return None;
        }
        found = Some(value.to_owned());
    }
    found
}

fn cookie_tokens(header: &str) -> Vec<String> {
    if header.len() > MAX_COOKIE_HEADER_BYTES {
        return Vec::new();
    }
    let mut tokens = Vec::new();
    for part in header.split(';') {
        let Some((name, value)) = part.trim().split_once('=') else {
            continue;
        };
        if name == COOKIE && valid_token(value) && !tokens.iter().any(|token| token == value) {
            tokens.push(value.to_owned());
        }
    }
    tokens
}

fn valid_token(value: &str) -> bool {
    value.len() <= 64
        && URL_SAFE_NO_PAD
            .decode(value)
            .is_ok_and(|bytes| bytes.len() == TOKEN_BYTES)
}

fn token_key(token: &str) -> Vec<u8> {
    Sha256::digest(token.as_bytes()).to_vec()
}

fn random_token(size: usize) -> Result<String, String> {
    let mut bytes = vec![0; size];
    getrandom::fill(&mut bytes).map_err(display)?;
    Ok(URL_SAFE_NO_PAD.encode(bytes))
}

fn encode(value: &StoredSession) -> Result<Vec<u8>, String> {
    serde_json::to_vec(value).map_err(display)
}

fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}

#[cfg(test)]
mod tests {
    use super::{cookie_token, cookie_tokens};

    #[test]
    fn ignores_missing_and_malformed_session_cookies() {
        assert_eq!(cookie_token("theme=dark"), None);
        assert_eq!(cookie_token("rc_session=not-base64"), None);
    }

    #[test]
    fn rejects_duplicate_session_cookies_for_lookup() {
        let token = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA";
        assert_eq!(
            cookie_token(&format!("rc_session={token}; rc_session={token}")),
            None
        );
    }

    #[test]
    fn collects_every_valid_session_cookie_for_revocation() {
        let first = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA";
        let second = "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE";
        assert_eq!(
            cookie_tokens(&format!(
                "rc_session={first}; rc_session=invalid; rc_session={second}; rc_session={first}"
            )),
            [first.to_owned(), second.to_owned()]
        );
    }
}
