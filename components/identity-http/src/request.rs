use crate::ohrats::rc_http::types::Request;
use serde::de::DeserializeOwned;
use std::collections::BTreeMap;

pub fn header<'a>(request: &'a Request, name: &str) -> Option<&'a str> {
    request
        .headers
        .iter()
        .find(|header| header.name.eq_ignore_ascii_case(name))
        .map(|header| header.value.as_str())
}

pub fn cookie_header(request: &Request) -> &str {
    header(request, "cookie").unwrap_or_default()
}

pub fn json<T: DeserializeOwned>(request: &Request) -> Result<T, String> {
    if request.body.len() > 1024 * 1024 {
        return Err("request body is too large".into());
    }
    serde_json::from_slice(&request.body).map_err(|_| "invalid request".into())
}

pub fn query(request: &Request) -> BTreeMap<String, String> {
    request
        .query
        .split('&')
        .filter(|part| !part.is_empty())
        .filter_map(|part| {
            let (key, value) = part.split_once('=').unwrap_or((part, ""));
            Some((decode(key)?, decode(value)?))
        })
        .collect()
}

pub fn safe_next(value: &str) -> bool {
    value.starts_with('/')
        && !value.starts_with("//")
        && value.len() <= 2048
        && !value.chars().any(char::is_control)
}

fn decode(value: &str) -> Option<String> {
    let bytes = value.as_bytes();
    let mut result = Vec::with_capacity(bytes.len());
    let mut index = 0;
    while index < bytes.len() {
        match bytes[index] {
            b'+' => result.push(b' '),
            b'%' if index + 2 < bytes.len() => {
                result.push((hex(bytes[index + 1])? << 4) | hex(bytes[index + 2])?);
                index += 2;
            }
            b'%' => return None,
            byte => result.push(byte),
        }
        index += 1;
    }
    String::from_utf8(result).ok()
}

fn hex(value: u8) -> Option<u8> {
    match value {
        b'0'..=b'9' => Some(value - b'0'),
        b'a'..=b'f' => Some(value - b'a' + 10),
        b'A'..=b'F' => Some(value - b'A' + 10),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::{decode, safe_next};

    #[test]
    fn decodes_query_values_and_rejects_external_redirects() {
        assert_eq!(decode("a%2Fb+c").as_deref(), Some("a/b c"));
        assert!(safe_next("/devices?x=1"));
        assert!(!safe_next("//attacker.test"));
        assert!(!safe_next("https://attacker.test"));
    }
}
