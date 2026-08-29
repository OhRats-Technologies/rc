use crate::model::{StoredMember, StoredSnapshot};
use crate::ohrats::rc_authority::types::Snapshot;

pub fn snapshot(value: &Snapshot) -> Result<StoredSnapshot, String> {
    let stored = StoredSnapshot::from(value);
    if stored.version != 1 || stored.workspace_id.is_empty() {
        return Err("version and workspace-id are required".into());
    }
    if !strict_ids(stored.devices.iter().map(|v| v.id.as_str())) {
        return Err("devices must be sorted by unique id".into());
    }
    for device in &stored.devices {
        if device.id.is_empty()
            || device.identity_public_key.len() != 32
            || device.transport_public_key.len() != 32
        {
            return Err("device identity and transport keys must be 32 bytes".into());
        }
    }
    if stored.members.is_empty() || !strict_ids(stored.members.iter().map(|v| v.user_id.as_str())) {
        return Err("members must be non-empty and sorted by unique user-id".into());
    }
    if !stored.members.iter().any(|member| member.role.is_owner()) {
        return Err("at least one Owner is required".into());
    }
    for member in &stored.members {
        validate_member(member)?;
    }
    if !strict_ids(stored.api_keys.iter().map(|v| v.id.as_str())) {
        return Err("API keys must be sorted by unique id".into());
    }
    for key in &stored.api_keys {
        if key.id.is_empty() || key.user_id.is_empty() || key.public_key.len() != 32 {
            return Err("API key fields are invalid".into());
        }
        if !strict_strings(key.scopes.iter().map(String::as_str)) {
            return Err("API key scopes must be sorted and unique".into());
        }
    }
    if !strict_ids(
        stored
            .active_execution_mcp_grants
            .iter()
            .map(|v| v.id.as_str()),
    ) {
        return Err("MCP grants must be sorted by unique id".into());
    }
    for grant in &stored.active_execution_mcp_grants {
        if grant.id.is_empty() || grant.user_id.is_empty() || !hex_hash(&grant.hash) {
            return Err("MCP grant fields are invalid".into());
        }
    }
    Ok(stored)
}

fn validate_member(member: &StoredMember) -> Result<(), String> {
    if member.user_id.is_empty() {
        return Err("member user-id is required".into());
    }
    if !strict_ids(member.passkeys.iter().map(|v| v.credential_id.as_str())) {
        return Err("passkeys must be sorted by unique credential-id".into());
    }
    for passkey in &member.passkeys {
        if passkey.credential_id.is_empty() || passkey.public_key.is_empty() {
            return Err("passkey fields are invalid".into());
        }
    }
    if !strict_ids(member.control_keys.iter().map(|v| v.id.as_str())) {
        return Err("control keys must be sorted by unique id".into());
    }
    for key in &member.control_keys {
        if key.id.is_empty() || key.public_key.len() != 32 {
            return Err("control key fields are invalid".into());
        }
        if !member
            .passkeys
            .iter()
            .any(|passkey| passkey.credential_id == key.authorized_by_passkey)
        {
            return Err("control key is not passkey-authorized".into());
        }
    }
    Ok(())
}

fn strict_ids<'a>(values: impl Iterator<Item = &'a str>) -> bool {
    strict_strings(values)
}

fn strict_strings<'a>(mut values: impl Iterator<Item = &'a str>) -> bool {
    let Some(mut previous) = values.next() else {
        return true;
    };
    if previous.is_empty() {
        return false;
    }
    for value in values {
        if value.is_empty() || previous >= value {
            return false;
        }
        previous = value;
    }
    true
}

fn hex_hash(value: &str) -> bool {
    value.len() == 64
        && value
            .bytes()
            .all(|byte| byte.is_ascii_hexdigit() && !byte.is_ascii_uppercase())
}
