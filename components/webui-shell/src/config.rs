use crate::ohrats::rc_http::types::Request;
use crate::ohrats::rc_plugin::state_store;

const PUBLIC_SIGNUP: &str = "public-signup";
const PUBLIC_URL: &str = "public-url";

pub fn public_signup() -> bool {
    state_store::read(PUBLIC_SIGNUP)
        .ok()
        .flatten()
        .is_some_and(|value| value == b"true")
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

pub fn invoke(args: &[String]) -> Result<u32, String> {
    match args {
        [] => {
            println!("public-signup\t{}", public_signup());
            let url = state_store::read(PUBLIC_URL)?
                .and_then(|value| String::from_utf8(value).ok())
                .unwrap_or_else(|| "auto".into());
            println!("public-url\t{url}");
            Ok(0)
        }
        [key, value] if key == PUBLIC_SIGNUP && matches!(value.as_str(), "true" | "false") => {
            state_store::write(PUBLIC_SIGNUP, value.as_bytes())?;
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
        _ => Err("usage: rc webui-config [public-signup true|false|public-url URL|auto]".into()),
    }
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

#[cfg(test)]
mod tests {
    use super::{valid_authority, valid_url};

    #[test]
    fn validates_public_origins_and_forwarded_authorities() {
        assert!(valid_url("https://rc.ohrats.party"));
        assert!(!valid_url("javascript:alert(1)"));
        assert!(valid_authority("rc.ohrats.party"));
        assert!(!valid_authority("rc.ohrats.party/evil"));
    }
}
