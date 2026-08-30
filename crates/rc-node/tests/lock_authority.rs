use rc_node::{
    LockError, api_control_authority, bootstrap_lock, load_lock, lock_metadata, schedule_authority,
};
use rc_protocol::{AuthorityApiKey, AuthorityMember, AuthorityScheduleGrant, AuthoritySnapshot};
use std::time::{SystemTime, UNIX_EPOCH};

fn snapshot(expires_at: i64) -> String {
    serde_json::to_string(&AuthoritySnapshot {
        v: 1,
        workspace_id: "workspace".into(),
        devices: Vec::new(),
        members: vec![AuthorityMember {
            user_id: "user".into(),
            role: "owner".into(),
            credentials: Vec::new(),
        }],
        api_keys: vec![AuthorityApiKey {
            id: "api".into(),
            user_id: "user".into(),
            public_key: "public-key".into(),
            scopes: vec!["execute".into(), "manage-devices".into()],
            expires_at,
        }],
        mcp_grants: Vec::new(),
        schedule_grants: Vec::new(),
    })
    .unwrap()
}

#[test]
fn lock_bootstrap_is_tofu_and_reports_metadata() -> anyhow::Result<()> {
    let dir = temp_dir("bootstrap");
    let first = snapshot(0);
    bootstrap_lock(&dir, &first, "https://rc.example.test:8443/path")?;
    let locked = load_lock(&dir)?;
    assert_eq!(locked.snapshot, first);
    assert_eq!(locked.origin, "https://rc.example.test:8443");
    assert_eq!(locked.rp_id, "rc.example.test");
    let (hash, generation) = lock_metadata(&dir);
    assert_eq!(hash.len(), 64);
    assert_eq!(generation, 0);

    let second = serde_json::to_string(&AuthoritySnapshot {
        v: 1,
        workspace_id: "other".into(),
        devices: Vec::new(),
        members: Vec::new(),
        api_keys: Vec::new(),
        mcp_grants: Vec::new(),
        schedule_grants: Vec::new(),
    })?;
    bootstrap_lock(&dir, &second, "https://evil.invalid")?;
    assert_eq!(load_lock(&dir)?.snapshot, first);
    let _ = std::fs::remove_dir_all(dir);
    Ok(())
}

#[test]
fn corrupt_existing_lock_is_never_replaced() -> anyhow::Result<()> {
    let dir = temp_dir("corrupt");
    std::fs::create_dir_all(&dir)?;
    std::fs::write(dir.join("lock.json"), b"not-json")?;
    assert!(matches!(
        bootstrap_lock(&dir, &snapshot(0), "https://rc.example.test"),
        Err(LockError::Corrupt)
    ));
    assert_eq!(std::fs::read(dir.join("lock.json"))?, b"not-json");
    let _ = std::fs::remove_dir_all(dir);
    Ok(())
}

#[test]
fn api_control_authority_rejects_expired_keys() -> anyhow::Result<()> {
    let dir = temp_dir("api-expiry");
    let expired = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
        - 1;
    bootstrap_lock(&dir, &snapshot(expired), "https://rc.example.test")?;
    assert!(matches!(
        api_control_authority(&dir, "api"),
        Err(LockError::ApiKey)
    ));
    let _ = std::fs::remove_dir_all(dir);

    let dir = temp_dir("api-active");
    bootstrap_lock(&dir, &snapshot(0), "https://rc.example.test")?;
    let authority = api_control_authority(&dir, "api")?;
    assert_eq!(authority.user_id, "user");
    assert_eq!(authority.role, "owner");
    assert!(authority.can_execute);
    assert!(authority.can_manage_devices);
    let _ = std::fs::remove_dir_all(dir);
    Ok(())
}

#[test]
fn schedule_authority_binds_device_owner_and_spec_hash() -> anyhow::Result<()> {
    let dir = temp_dir("schedule");
    let mut value: AuthoritySnapshot = serde_json::from_str(&snapshot(0))?;
    value.schedule_grants.push(AuthorityScheduleGrant {
        schedule_id: "schedule-1".into(),
        device_id: "device-1".into(),
        user_id: "user".into(),
        spec_hash: "sha256:fixture".into(),
        max_runtime_ms: 60_000,
        expires_at: 0,
    });
    bootstrap_lock(
        &dir,
        &serde_json::to_string(&value)?,
        "https://rc.example.test",
    )?;
    let grant = schedule_authority(&dir, "schedule-1", "device-1", "sha256:fixture")?;
    assert_eq!(grant.max_runtime_ms, 60_000);
    assert!(matches!(
        schedule_authority(&dir, "schedule-1", "device-1", "sha256:wrong"),
        Err(LockError::ScheduleGrant)
    ));
    let _ = std::fs::remove_dir_all(dir);
    Ok(())
}

fn temp_dir(label: &str) -> std::path::PathBuf {
    std::env::temp_dir().join(format!(
        "rc-lock-{label}-{}-{}",
        std::process::id(),
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_nanos()
    ))
}
