use crate::ohrats::rc_authority::types::{
    ControlKey, Device, ExecutionGrant, Member, Passkey, Role, Snapshot,
};

pub fn snapshot(fixture: &str, control_public_key: Vec<u8>) -> Snapshot {
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
            control_keys: vec![ControlKey {
                id: "fixture-control".into(),
                public_key: control_public_key,
                authorized_by_passkey: "fixture-passkey".into(),
            }],
        }],
        api_keys: Vec::new(),
        active_execution_mcp_grants: Vec::new(),
    }
}

pub fn transitioned(mut snapshot: Snapshot) -> Snapshot {
    snapshot.active_execution_mcp_grants = vec![ExecutionGrant {
        id: "fixture-grant".into(),
        user_id: "fixture-owner".into(),
        hash: "00".repeat(32),
    }];
    snapshot
}

pub fn decode_hex(value: &str, expected: usize) -> Result<Vec<u8>, String> {
    if value.len() != expected * 2 || !value.bytes().all(|byte| byte.is_ascii_hexdigit()) {
        return Err("invalid fixture hex".into());
    }
    (0..expected)
        .map(|index| {
            u8::from_str_radix(&value[index * 2..index * 2 + 2], 16).map_err(|e| e.to_string())
        })
        .collect()
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
