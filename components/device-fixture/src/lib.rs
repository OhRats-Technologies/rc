wit_bindgen::generate!({
    path: "../../wit",
    world: "device-fixture",
    generate_all,
});

use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use ohrats::{
    rc_devices::{
        enrollments, presence, registry,
        types::{EnrollmentError, EnrollmentInput, NodeStatus, NodeUpdate},
    },
    rc_plugin::types::{Command, Requirement, Selection},
};
use std::time::{SystemTime, UNIX_EPOCH};

struct DeviceFixture;

impl Guest for DeviceFixture {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:device-fixture".into(),
            version: "0.1.0".into(),
            provides: Vec::new(),
            requires: vec![
                requirement("ohrats:rc-devices/registry"),
                requirement("ohrats:rc-devices/enrollments"),
                requirement("ohrats:rc-devices/presence"),
            ],
            commands: vec![
                command("devices-seed", "Seed device state", "rc devices-seed <id>"),
                command(
                    "devices-verify",
                    "Verify persistent device state",
                    "rc devices-verify <id> <device-id>",
                ),
            ],
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }

    fn deactivate() {}

    fn invoke(command: String, args: Vec<String>) -> Result<u32, String> {
        match command.as_str() {
            "devices-seed" => seed(&args),
            "devices-verify" => verify(&args),
            _ => Err(format!("unsupported command {command:?}")),
        }
    }
}

fn seed(args: &[String]) -> Result<u32, String> {
    let [fixture] = args else {
        return Err("usage: rc devices-seed <id>".into());
    };
    valid_fixture(fixture)?;
    let now = now_ms();
    let workspace = workspace(fixture);
    let issued = enrollments::issue(&workspace, "fixture-owner", now, now + 60_000)?;
    let first = enrollments::consume(&issued.token, now + 1, &input(1, "Original"))
        .map_err(enrollment_error)?;
    let retry = enrollments::consume(&issued.token, now + 2, &input(1, "Changed"))
        .map_err(enrollment_error)?;
    if retry.id != first.id || retry.name != "Original" {
        return Err("same-identity enrollment retry was not idempotent".into());
    }
    if !matches!(
        enrollments::consume(&issued.token, now + 3, &input(2, "Wrong retry")),
        Err(EnrollmentError::TokenUsed)
    ) {
        return Err("used token accepted a different identity".into());
    }
    duplicate_retry(&workspace, now, &first.id)?;
    let status = presence::renew(
        &first.id,
        &key(1),
        now + 10,
        now + 1000,
        &update("host-renewed", "rendezvous-fixture"),
    )?;
    let NodeStatus::Active(active) = status else {
        return Err("active device did not renew presence".into());
    };
    if active.transport_public_key != key(101) || active.hostname != "host-renewed" {
        return Err("presence update changed immutable keys or lost metadata".into());
    }
    let online = presence::get(&first.id, now + 20)?.ok_or("presence missing")?;
    let offline = presence::get(&first.id, now + 1001)?.ok_or("presence missing")?;
    if !online.online || offline.online || offline.rendezvous.is_some() {
        return Err("presence lease expiry is incorrect".into());
    }
    let renamed = registry::rename(&first.id, "Renamed fixture")?;
    if renamed.name != "Renamed fixture" || registry::all(Some(&workspace))?.len() != 2 {
        return Err("device rename or workspace listing failed".into());
    }
    println!("{} {}", issued.token, first.id);
    Ok(0)
}

fn duplicate_retry(workspace: &str, now: u64, existing: &str) -> Result<(), String> {
    let issued = enrollments::issue(workspace, "fixture-owner", now + 4, now + 60_000)?;
    if !matches!(
        enrollments::consume(&issued.token, now + 5, &input(1, "Duplicate")),
        Err(EnrollmentError::DuplicateIdentity(id)) if id == existing
    ) {
        return Err("duplicate identity was accepted".into());
    }
    enrollments::consume(&issued.token, now + 6, &input(2, "Retry device"))
        .map_err(enrollment_error)?;
    Ok(())
}

fn verify(args: &[String]) -> Result<u32, String> {
    let [fixture, device_id] = args else {
        return Err("usage: rc devices-verify <id> <device-id>".into());
    };
    valid_fixture(fixture)?;
    let now = now_ms();
    let device = registry::get(device_id)?.ok_or("device did not survive restart")?;
    if device.name != "Renamed fixture" || device.identity_public_key != key(1) {
        return Err("persistent device identity is invalid".into());
    }
    let tombstone = registry::revoke(device_id, now)?.ok_or("revoke failed")?;
    if tombstone.identity_public_key != key(1)
        || registry::get(device_id)?.is_some()
        || presence::get(device_id, now)?.is_some()
    {
        return Err("revocation was not atomic".into());
    }
    if !matches!(
        registry::resolve_node(device_id, &key(1))?,
        NodeStatus::Revoked(_)
    ) || !matches!(
        registry::resolve_node(device_id, &key(9))?,
        NodeStatus::Unknown
    ) {
        return Err("tombstone status did not preserve 410/404 distinction".into());
    }
    if !matches!(
        presence::renew(
            device_id,
            &key(1),
            now + 1,
            now + 1000,
            &update("offline", "blocked")
        )?,
        NodeStatus::Revoked(_)
    ) {
        return Err("revoked offline Node reconnected".into());
    }
    verify_device_limit(&workspace(fixture), now)?;
    println!("device state: ok");
    Ok(0)
}

fn verify_device_limit(workspace: &str, now: u64) -> Result<(), String> {
    let existing = registry::all(Some(workspace))?.len();
    let overflow = enrollments::issue(workspace, "fixture-owner", now, now + 60_000)?;
    for index in existing..25 {
        let issued =
            enrollments::issue(workspace, "fixture-owner", now + index as u64, now + 60_000)?;
        enrollments::consume(
            &issued.token,
            now + index as u64 + 1,
            &input(index as u8 + 10, "Limit"),
        )
        .map_err(enrollment_error)?;
    }
    if !matches!(
        enrollments::consume(&overflow.token, now + 101, &input(99, "Overflow")),
        Err(EnrollmentError::DeviceLimit)
    ) {
        return Err("workspace device limit was not enforced".into());
    }
    if enrollments::issue(workspace, "fixture-owner", now + 102, now + 60_000).is_ok() {
        return Err("enrollment issuance ignored the workspace device limit".into());
    }
    Ok(())
}

fn input(seed: u8, name: &str) -> EnrollmentInput {
    EnrollmentInput {
        name: name.into(),
        hostname: format!("host-{seed}"),
        platform: "darwin".into(),
        arch: "arm64".into(),
        identity_public_key: key(seed),
        transport_public_key: key(seed.wrapping_add(100)),
        version: "0.18.0".into(),
        capabilities: vec!["webrtc".into(), "process".into()],
    }
}

fn update(hostname: &str, rendezvous: &str) -> NodeUpdate {
    NodeUpdate {
        hostname: hostname.into(),
        platform: "darwin".into(),
        arch: "arm64".into(),
        version: "0.18.1".into(),
        capabilities: vec!["process".into(), "webrtc".into()],
        lock_hash: "abcdef".into(),
        lock_generation: 7,
        rendezvous: Some(rendezvous.into()),
    }
}

fn key(seed: u8) -> String {
    URL_SAFE_NO_PAD.encode([seed; 32])
}

fn enrollment_error(error: EnrollmentError) -> String {
    format!("enrollment failed: {error:?}")
}

fn workspace(fixture: &str) -> String {
    format!("fixture-workspace-{fixture}")
}

fn valid_fixture(value: &str) -> Result<(), String> {
    if !value.is_empty()
        && value.len() <= 40
        && value
            .bytes()
            .all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'-')
    {
        Ok(())
    } else {
        Err("invalid fixture id".into())
    }
}

fn now_ms() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_or(1, |value| value.as_millis() as u64)
}

fn requirement(name: &str) -> Requirement {
    Requirement {
        name: name.into(),
        version: "^0.1".into(),
        selection: Selection::Single,
    }
}

fn command(name: &str, summary: &str, usage: &str) -> Command {
    Command {
        name: name.into(),
        summary: summary.into(),
        usage: usage.into(),
    }
}

export!(DeviceFixture);
