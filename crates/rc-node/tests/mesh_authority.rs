use rc_node::{MeshAuthority, NodeState, bootstrap_lock};
use rc_protocol::{AuthorityDevice, AuthoritySnapshot};
use std::time::{SystemTime, UNIX_EPOCH};

fn snapshot(workspace: &str, devices: Vec<AuthorityDevice>) -> String {
    serde_json::to_string(&AuthoritySnapshot {
        v: 1,
        workspace_id: workspace.into(),
        devices,
        members: Vec::new(),
        api_keys: Vec::new(),
        mcp_grants: Vec::new(),
        schedule_grants: Vec::new(),
    })
    .unwrap()
}

#[test]
fn legacy_locks_bootstrap_only_the_local_mesh_identity() -> anyhow::Result<()> {
    let root = temp_dir("legacy");
    let state = NodeState::generate("local".into());
    bootstrap_lock(&root, &snapshot("workspace", Vec::new()), "https://rc.test")?;
    let authority = MeshAuthority::from_lock(&state, &root)?;
    assert_eq!(authority.local_device_id(), "local");
    assert_eq!(authority.devices().count(), 1);
    std::fs::remove_dir_all(root)?;
    Ok(())
}

#[test]
fn signed_device_directories_pin_local_and_remote_keys() -> anyhow::Result<()> {
    let root = temp_dir("directory");
    let local = NodeState::generate("local".into());
    let remote = NodeState::generate("remote".into());
    let devices = vec![
        AuthorityDevice {
            id: local.device_id.clone(),
            identity_public_key: local.identity_public_key()?,
            transport_public_key: local.transport_public_key()?,
        },
        AuthorityDevice {
            id: remote.device_id.clone(),
            identity_public_key: remote.identity_public_key()?,
            transport_public_key: remote.transport_public_key()?,
        },
    ];
    bootstrap_lock(&root, &snapshot("workspace", devices), "https://rc.test")?;
    let authority = MeshAuthority::from_lock(&local, &root)?;
    assert_eq!(authority.devices().count(), 2);
    assert!(authority.device("remote").is_some());
    std::fs::remove_dir_all(root)?;
    Ok(())
}

#[test]
fn a_lock_cannot_replace_the_local_mesh_identity() -> anyhow::Result<()> {
    let root = temp_dir("impostor");
    let local = NodeState::generate("local".into());
    let impostor = NodeState::generate("local".into());
    let devices = vec![AuthorityDevice {
        id: "local".into(),
        identity_public_key: impostor.identity_public_key()?,
        transport_public_key: impostor.transport_public_key()?,
    }];
    bootstrap_lock(&root, &snapshot("workspace", devices), "https://rc.test")?;
    assert!(MeshAuthority::from_lock(&local, &root).is_err());
    std::fs::remove_dir_all(root)?;
    Ok(())
}

fn temp_dir(label: &str) -> std::path::PathBuf {
    std::env::temp_dir().join(format!(
        "rc-mesh-authority-{label}-{}-{}",
        std::process::id(),
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_nanos()
    ))
}
