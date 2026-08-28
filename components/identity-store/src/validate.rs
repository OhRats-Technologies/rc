const MAX_ID_BYTES: usize = 128;
const MAX_KIND_BYTES: usize = 64;
const MAX_NAME_BYTES: usize = 120;
const MAX_METADATA_BYTES: usize = 64 * 1024;
const MAX_STATE_BYTES: usize = 512 * 1024;

pub fn id(value: &str, label: &str) -> Result<(), String> {
    if !value.is_empty()
        && value.len() <= MAX_ID_BYTES
        && value
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'.' | b'-' | b'_' | b':'))
    {
        Ok(())
    } else {
        Err(format!("invalid {label}"))
    }
}

pub fn kind(value: &str) -> Result<(), String> {
    if !value.is_empty()
        && value.len() <= MAX_KIND_BYTES
        && value
            .bytes()
            .all(|byte| byte.is_ascii_lowercase() || byte.is_ascii_digit() || byte == b'-')
    {
        Ok(())
    } else {
        Err("invalid ceremony kind".into())
    }
}

pub fn display_name(value: &str) -> Result<(), String> {
    if !value.trim().is_empty()
        && value.len() <= MAX_NAME_BYTES
        && !value.chars().any(char::is_control)
    {
        Ok(())
    } else {
        Err("invalid user display name".into())
    }
}

pub fn ceremony_payload(metadata: &[u8], state: &[u8]) -> Result<(), String> {
    if metadata.len() > MAX_METADATA_BYTES {
        return Err("ceremony metadata is too large".into());
    }
    if state.is_empty() || state.len() > MAX_STATE_BYTES {
        return Err("invalid ceremony state".into());
    }
    Ok(())
}
