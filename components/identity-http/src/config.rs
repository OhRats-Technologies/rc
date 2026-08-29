use crate::ohrats::{rc_http::types::Request, rc_plugin::state_store};
use sha2::{Digest, Sha256};

const PUBLIC_URL: &str = "public-url";
const SETUP_TOKEN_HASH: &str = "setup-token-hash";

pub struct RelyingParty {
    pub id: String,
    pub origin: String,
    pub secure: bool,
}

pub fn relying_party(request: &Request) -> Result<RelyingParty, String> {
    let origin = public_url(request);
    let (scheme, rest) = origin
        .split_once("://")
        .ok_or_else(|| "invalid identity public URL".to_owned())?;
    let authority = rest
        .split('/')
        .next()
        .ok_or_else(|| "invalid identity public URL".to_owned())?;
    let id = if authority.starts_with('[') {
        authority
            .split(']')
            .next()
            .map(|value| format!("{value}]"))
            .unwrap_or_default()
    } else {
        authority.split(':').next().unwrap_or_default().to_owned()
    };
    if id.is_empty() || id.len() > 255 {
        return Err("invalid identity relying-party id".into());
    }
    let secure = scheme == "https";
    if !secure && id != "localhost" {
        return Err("passkeys require HTTPS or localhost".into());
    }
    Ok(RelyingParty { id, origin, secure })
}

pub fn public_url(request: &Request) -> String {
    if let Ok(Some(value)) = state_store::read(PUBLIC_URL)
        && let Ok(value) = String::from_utf8(value)
        && valid_url(&value)
    {
        return value.trim_end_matches('/').to_owned();
    }
    let scheme = forwarded(request, "x-forwarded-proto")
        .filter(|value| matches!(value.as_str(), "http" | "https"))
        .unwrap_or_else(|| request.scheme.clone());
    let authority = forwarded(request, "x-forwarded-host")
        .filter(|value| valid_authority(value))
        .unwrap_or_else(|| request.authority.clone());
    format!("{scheme}://{authority}")
        .trim_end_matches('/')
        .to_owned()
}

pub fn setup_authorized(cookie_header: &str) -> Result<bool, String> {
    let Some(expected) = state_store::read(SETUP_TOKEN_HASH)? else {
        return Ok(true);
    };
    Ok(cookie(cookie_header, "rc_setup").is_some_and(|value| value == hex(&expected)))
}

pub fn setup_cookie(token: &str, secure: bool) -> Result<Option<String>, String> {
    let Some(expected) = state_store::read(SETUP_TOKEN_HASH)? else {
        return Ok(None);
    };
    if digest(token.as_bytes()).as_slice() != expected.as_slice() {
        return Err("invalid setup link".into());
    }
    let value = hex(&digest(token.as_bytes()));
    Ok(Some(format!(
        "rc_setup={value}; Path=/; HttpOnly; SameSite=Strict; Max-Age=900{}",
        if secure { "; Secure" } else { "" }
    )))
}

pub fn invoke(args: &[String]) -> Result<u32, String> {
    match args {
        [] => {
            let url = state_store::read(PUBLIC_URL)?
                .and_then(|value| String::from_utf8(value).ok())
                .unwrap_or_else(|| "auto".into());
            println!("public-url\t{url}");
            println!(
                "setup-token\t{}",
                if state_store::read(SETUP_TOKEN_HASH)?.is_some() {
                    "configured"
                } else {
                    "none"
                }
            );
            Ok(0)
        }
        [key, value] if key == PUBLIC_URL && value == "auto" => {
            state_store::remove(PUBLIC_URL)?;
            Ok(0)
        }
        [key, value] if key == PUBLIC_URL && valid_url(value) => {
            state_store::write(PUBLIC_URL, value.trim_end_matches('/').as_bytes())?;
            Ok(0)
        }
        [key, value] if key == "setup-token" && value == "none" => {
            state_store::remove(SETUP_TOKEN_HASH)?;
            Ok(0)
        }
        [key, value] if key == "setup-token" && valid_setup_token(value) => {
            state_store::write(SETUP_TOKEN_HASH, &digest(value.as_bytes()))?;
            Ok(0)
        }
        _ => Err("usage: rc identity-config [public-url URL|auto|setup-token TOKEN|none]".into()),
    }
}

pub fn cookie<'a>(header: &'a str, name: &str) -> Option<&'a str> {
    header
        .split(';')
        .filter_map(|part| part.trim().split_once('='))
        .find(|(key, _)| *key == name)
        .map(|(_, value)| value)
}

fn forwarded(request: &Request, name: &str) -> Option<String> {
    request
        .headers
        .iter()
        .find(|header| header.name.eq_ignore_ascii_case(name))
        .and_then(|header| header.value.split(',').next())
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
}

fn valid_url(value: &str) -> bool {
    (value.starts_with("https://") || value.starts_with("http://"))
        && !value
            .bytes()
            .any(|byte| byte.is_ascii_control() || byte == b' ')
}

fn valid_authority(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 255
        && !value
            .bytes()
            .any(|byte| byte.is_ascii_control() || matches!(byte, b' ' | b'/' | b'\\'))
}

fn valid_setup_token(value: &str) -> bool {
    value.len() >= 24 && value.len() <= 256 && value.bytes().all(|byte| byte.is_ascii_graphic())
}

fn digest(value: &[u8]) -> Vec<u8> {
    Sha256::digest(value).to_vec()
}

fn hex(value: &[u8]) -> String {
    value.iter().map(|byte| format!("{byte:02x}")).collect()
}

#[cfg(test)]
mod tests {
    use super::{valid_authority, valid_setup_token, valid_url};

    #[test]
    fn validates_identity_configuration() {
        assert!(valid_url("https://rc.ohrats.party"));
        assert!(!valid_url("javascript:alert(1)"));
        assert!(valid_authority("rc.ohrats.party"));
        assert!(!valid_authority("rc.ohrats.party/evil"));
        assert!(valid_setup_token("012345678901234567890123"));
        assert!(!valid_setup_token("short"));
    }
}
