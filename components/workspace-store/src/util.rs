use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use sha2::{Digest, Sha256};
use std::time::{SystemTime, UNIX_EPOCH};

pub const WORKSPACES: &str = "workspaces";
pub const MEMBERS: &str = "workspace-members";
pub const USER_MEMBERS: &str = "workspace-members-by-user";
pub const PERSONAL: &str = "personal-workspaces";
pub const INVITES: &str = "workspace-invitations";
pub const INVITE_IDS: &str = "workspace-invitations-by-id";
pub const MAX_MEMBERS: usize = 256;
pub const MAX_INVITATIONS: usize = 128;
pub const MAX_USER_WORKSPACES: usize = 256;

pub fn now_ms() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_or(0, |v| v.as_millis() as u64)
}
pub fn encode<T: serde::Serialize>(v: &T) -> Result<Vec<u8>, String> {
    serde_json::to_vec(v).map_err(|e| e.to_string())
}
pub fn decode<T: serde::de::DeserializeOwned>(v: &[u8]) -> Result<T, String> {
    serde_json::from_slice(v).map_err(|e| e.to_string())
}
pub fn pair(a: &str, b: &str) -> Vec<u8> {
    let mut v = a.as_bytes().to_vec();
    v.push(0);
    v.extend_from_slice(b.as_bytes());
    v
}
pub fn prefix(a: &str) -> Vec<u8> {
    let mut v = a.as_bytes().to_vec();
    v.push(0);
    v
}
pub fn digest(parts: &[&[u8]]) -> Vec<u8> {
    let mut h = Sha256::new();
    for p in parts {
        h.update((p.len() as u64).to_be_bytes());
        h.update(p);
    }
    h.finalize().to_vec()
}
pub fn id(parts: &[&[u8]]) -> String {
    URL_SAFE_NO_PAD.encode(&digest(parts)[..18])
}
pub fn random(size: usize) -> Result<Vec<u8>, String> {
    let mut v = vec![0; size];
    getrandom::fill(&mut v).map_err(|e| e.to_string())?;
    Ok(v)
}
pub fn token() -> Result<String, String> {
    Ok(URL_SAFE_NO_PAD.encode(random(32)?))
}
pub fn token_key(token: &str) -> Option<Vec<u8>> {
    URL_SAFE_NO_PAD
        .decode(token)
        .ok()
        .filter(|v| v.len() == 32)
        .map(|_| Sha256::digest(token.as_bytes()).to_vec())
}

pub fn valid_id(value: &str, label: &str) -> Result<(), String> {
    if !value.is_empty()
        && value.len() <= 128
        && value
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || matches!(b, b'-' | b'_' | b'.' | b':'))
    {
        Ok(())
    } else {
        Err(format!("invalid {label}"))
    }
}
pub fn valid_name(value: &str) -> Result<(), String> {
    if !value.trim().is_empty() && value.len() <= 120 && !value.chars().any(char::is_control) {
        Ok(())
    } else {
        Err("invalid workspace name".into())
    }
}
