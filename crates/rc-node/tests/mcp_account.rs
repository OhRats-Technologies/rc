use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_node::{ControlAuthority, bootstrap_lock, snapshot_hash, verify_mcp_grant};
use rc_protocol::{AuthoritySnapshot, ControlGrant, McpGrantPayload};

fn check(
    v: u32,
    audience: Option<&str>,
    devices: &[&str],
    role: &str,
    included: bool,
    domain: u32,
    target: &str,
) -> anyhow::Result<bool> {
    let seed = URL_SAFE_NO_PAD.encode([42_u8; 32]);
    let signing = ed25519_dalek::SigningKey::from_bytes(&[42_u8; 32]);
    let grant = McpGrantPayload {
        v,
        audience: audience.map(str::to_owned),
        id: "mcp".into(),
        user_id: "owner".into(),
        client_id: "client".into(),
        client_name: "Agent".into(),
        device_ids: devices.iter().map(|id| (*id).into()).collect(),
        scopes: vec!["mcp:terminal".into()],
        issued_at: 1,
        expires_at: 0,
    };
    let encoded = serde_json::to_string(&grant)?;
    let digest = snapshot_hash(&encoded);
    let signature = rc_crypto::sign_ed25519_seed(
        &seed,
        format!("rc-mcp-grant-v{domain}\n{digest}").as_bytes(),
    )?;
    let snapshot: AuthoritySnapshot = serde_json::from_value(serde_json::json!({
        "v":1,"workspaceId":"workspace","members":[{"userId":"owner","role":role}],
        "mcpGrants": if included { vec![serde_json::json!({"id":"mcp","userId":"owner","hash":digest})] } else { vec![] }
    }))?;
    let control = ControlAuthority {
        role: "owner".into(),
        grant: ControlGrant {
            v: 1,
            client_id: "control".into(),
            user_id: "owner".into(),
            signing_public_key: URL_SAFE_NO_PAD.encode(signing.verifying_key().as_bytes()),
            issued_at: 1,
            expires_at: 0,
        },
    };
    let root = std::env::temp_dir().join(format!("rc-mcp-account-{}", rand::random::<u128>()));
    bootstrap_lock(
        &root,
        &serde_json::to_string(&snapshot)?,
        "https://rc.example",
    )?;
    let result = verify_mcp_grant(&root, &encoded, &signature, &control, "owner", target).is_ok();
    std::fs::remove_dir_all(root)?;
    Ok(result)
}

#[test]
fn account_grant_covers_future_devices_but_requires_local_owner_and_grant() -> anyhow::Result<()> {
    assert!(check(
        2,
        Some("account"),
        &[],
        "owner",
        true,
        2,
        "future-device"
    )?);
    assert!(!check(
        2,
        Some("account"),
        &[],
        "operator",
        true,
        2,
        "future-device"
    )?);
    assert!(!check(
        2,
        Some("account"),
        &[],
        "owner",
        false,
        2,
        "future-device"
    )?);
    assert!(!check(
        2,
        Some("account"),
        &[],
        "owner",
        true,
        1,
        "future-device"
    )?);
    Ok(())
}

#[test]
fn legacy_and_ambiguous_grants_never_gain_account_access() -> anyhow::Result<()> {
    assert!(check(1, None, &["original"], "owner", true, 1, "original")?);
    assert!(!check(
        1,
        None,
        &["original"],
        "owner",
        true,
        1,
        "future-device"
    )?);
    for (v, audience, devices) in [
        (1, Some("account"), vec!["original"]),
        (2, None, vec![]),
        (2, Some("account"), vec!["original"]),
        (3, Some("account"), vec![]),
    ] {
        assert!(!check(v, audience, &devices, "owner", true, v, "original")?);
    }
    Ok(())
}
