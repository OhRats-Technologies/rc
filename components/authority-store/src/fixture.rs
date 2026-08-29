use crate::ohrats::rc_authority::types::{Device, Member, Passkey, Role, Snapshot};

pub fn snapshot(fixture: &str) -> Snapshot {
    Snapshot {
        version: 1,
        workspace_id: format!("fixture-workspace-{fixture}"),
        devices: vec![Device {
            id: format!("fixture-device-{fixture}"),
            identity_public_key: vec![0x11; 32],
            transport_public_key: vec![0x22; 32],
        }],
        members: vec![Member {
            user_id: "fixture-owner".into(),
            role: Role::Owner,
            passkeys: vec![Passkey {
                credential_id: "fixture-passkey".into(),
                public_key: vec![0x33; 32],
            }],
            control_keys: Vec::new(),
        }],
        api_keys: Vec::new(),
        active_execution_mcp_grants: Vec::new(),
    }
}

pub fn validate_id(value: &str) -> Result<(), String> {
    if !value.is_empty()
        && value.len() <= 40
        && value
            .bytes()
            .all(|byte| byte.is_ascii_lowercase() || byte.is_ascii_digit() || byte == b'-')
    {
        Ok(())
    } else {
        Err("invalid fixture id".into())
    }
}
