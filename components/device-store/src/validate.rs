use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};

pub fn id(value: &str, label: &str) -> Result<(), String> {
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

pub fn text(value: &str, label: &str, limit: usize) -> Result<String, String> {
    let value = value.trim();
    if value.is_empty() || value.chars().count() > limit || value.contains('\0') {
        Err(format!("invalid {label}"))
    } else {
        Ok(value.into())
    }
}

pub fn key(value: &str, label: &str) -> Result<(), String> {
    if URL_SAFE_NO_PAD
        .decode(value)
        .is_ok_and(|decoded| decoded.len() == 32)
    {
        Ok(())
    } else {
        Err(format!("invalid {label}"))
    }
}

pub fn capabilities(values: &[String]) -> Result<Vec<String>, String> {
    if values.len() > 32 {
        return Err("too many capabilities".into());
    }
    let mut result = values.to_vec();
    result.sort();
    result.dedup();
    if result.iter().any(|value| {
        value.is_empty()
            || value.len() > 64
            || !value
                .bytes()
                .all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'-')
    }) {
        return Err("invalid capability".into());
    }
    Ok(result)
}

pub fn rendezvous(value: Option<String>) -> Result<Option<String>, String> {
    match value {
        Some(value) if value.trim().is_empty() || value.len() > 4096 || value.contains('\0') => {
            Err("invalid rendezvous metadata".into())
        }
        value => Ok(value),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn capabilities_are_canonical_and_bounded() {
        assert_eq!(
            capabilities(&["webrtc".into(), "process".into(), "webrtc".into()]).unwrap(),
            ["process", "webrtc"]
        );
        assert!(capabilities(&["SHELL".into()]).is_err());
    }
}
