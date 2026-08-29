use crate::ohrats::rc_api_credentials::types::{Lifetime, Scope};
use base64::Engine as _;

pub const MAX_ID: usize = 128;
pub const MAX_NAME: usize = 80;
pub const MAX_KEY: usize = 64;
pub const MAX_NONCE: usize = 128;
pub const MAX_SIGNATURE: usize = 128;
pub const MAX_CODE: usize = 256;

pub fn text(value: &str, label: &str, max: usize) -> Result<(), String> {
    if value.is_empty() || value.len() > max || value.chars().any(char::is_control) {
        Err(format!("invalid {label}"))
    } else {
        Ok(())
    }
}

pub fn id(value: &str, label: &str) -> Result<(), String> {
    text(value, label, MAX_ID)?;
    if value
        .bytes()
        .all(|b| b.is_ascii_alphanumeric() || b"._:-".contains(&b))
    {
        Ok(())
    } else {
        Err(format!("invalid {label}"))
    }
}

pub fn public_key(value: &str) -> Result<(), String> {
    text(value, "public key", MAX_KEY)?;
    let bytes = base64::engine::general_purpose::URL_SAFE_NO_PAD
        .decode(value)
        .map_err(|_| "invalid public key".to_owned())?;
    (bytes.len() == 32)
        .then_some(())
        .ok_or_else(|| "invalid public key".into())
}

pub fn scopes(value: &[Scope]) -> Vec<Scope> {
    let mut result = value.to_vec();
    result.sort_by_key(|scope| *scope as u8);
    result.dedup();
    if result.is_empty() {
        result.push(Scope::Read);
    }
    result
}

pub fn lifetime(value: Option<Lifetime>, now: u64) -> u64 {
    let age = match value.unwrap_or(Lifetime::Never) {
        Lifetime::Never => return 0,
        Lifetime::OneHour => 60 * 60,
        Lifetime::OneDay => 24 * 60 * 60,
        Lifetime::SevenDays => 7 * 24 * 60 * 60,
        Lifetime::ThirtyDays => 30 * 24 * 60 * 60,
        Lifetime::NinetyDays => 90 * 24 * 60 * 60,
        Lifetime::OneEightyDays => 180 * 24 * 60 * 60,
        Lifetime::OneYear => 365 * 24 * 60 * 60,
    };
    now.saturating_add(age * 1000)
}

pub fn nonce(value: &str) -> Result<(), String> {
    text(value, "nonce", MAX_NONCE)?;
    if !(16..=MAX_NONCE).contains(&value.len())
        || !value
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'_' || b == b'-')
    {
        return Err("invalid nonce".into());
    }
    Ok(())
}
