wit_bindgen::generate!({
    path: "../../wit",
    world: "identity-fixture",
    generate_all,
});

use ohrats::{
    rc_identity::{ceremonies, types::Ceremony, users},
    rc_plugin::types::{Command, Requirement, Selection},
    rc_session::{lookup, management},
};
use std::{
    thread,
    time::{Duration, SystemTime, UNIX_EPOCH},
};

const LIFETIME_MS: u64 = 60_000;
const EXPIRY_TEST_MS: u64 = 200;

struct IdentityFixture;

impl Guest for IdentityFixture {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:identity-fixture".into(),
            version: "0.1.0".into(),
            provides: Vec::new(),
            requires: vec![
                requirement("ohrats:rc-identity/users"),
                requirement("ohrats:rc-identity/ceremonies"),
                requirement("ohrats:rc-session/lookup"),
                requirement("ohrats:rc-session/management"),
            ],
            commands: vec![
                command(
                    "identity-seed",
                    "Create restart-persistent identity fixture state",
                    "rc identity-seed <fixture-id>",
                ),
                command(
                    "identity-verify",
                    "Verify and consume identity fixture state",
                    "rc identity-verify <fixture-id> <session-token>",
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
            "identity-seed" => seed(&args),
            "identity-verify" => verify(&args),
            _ => Err(format!("unsupported command {command:?}")),
        }
    }
}

fn seed(args: &[String]) -> Result<u32, String> {
    let [fixture] = args else {
        return Err("usage: rc identity-seed <fixture-id>".into());
    };
    validate_fixture(fixture)?;
    let user_id = user_id(fixture);
    let user = users::create(&user_id, "Identity Fixture")?;
    if user.id != user_id || user.display_name != "Identity Fixture" || user.created_at_ms == 0 {
        return Err("identity user creation returned invalid state".into());
    }
    if users::create(&user_id, "Duplicate").is_ok() {
        return Err("duplicate identity user was accepted".into());
    }
    ceremonies::put(&Ceremony {
        id: ceremony_id(fixture),
        kind: "registration".into(),
        user_id: Some(user_id.clone()),
        metadata: b"fixture-metadata".to_vec(),
        state: b"fixture-state".to_vec(),
        expires_at_ms: now_ms().saturating_add(LIFETIME_MS),
    })?;
    let issued = management::issue(&user_id, now_ms().saturating_add(LIFETIME_MS))?;
    if issued.session.principal.user_id != user_id || issued.token.is_empty() {
        return Err("browser session issuance returned invalid state".into());
    }
    println!("{}", issued.token);
    Ok(0)
}

fn verify(args: &[String]) -> Result<u32, String> {
    let [fixture, token] = args else {
        return Err("usage: rc identity-verify <fixture-id> <session-token>".into());
    };
    validate_fixture(fixture)?;
    let user_id = user_id(fixture);
    let cookie = format!("theme=dark; rc_session={token}; locale=en");
    let session =
        lookup::find(&cookie)?.ok_or_else(|| "browser session was not restored".to_owned())?;
    if session.principal.user_id != user_id
        || session.principal.display_name != "Identity Fixture"
        || session.created_at_ms == 0
        || session.expires_at_ms <= now_ms()
    {
        return Err("restored browser session is invalid".into());
    }
    let user = users::get(&user_id)?.ok_or_else(|| "identity user was not restored".to_owned())?;
    if user.display_name != "Identity Fixture" || users::count()? == 0 {
        return Err("restored identity user is invalid".into());
    }
    let expires_at_ms = now_ms().saturating_add(EXPIRY_TEST_MS);
    let expiring = management::issue(&user_id, expires_at_ms)?;
    let expiring_cookie = format!("rc_session={}", expiring.token);
    let expiring_id = format!("fixture-expiring-{fixture}");
    ceremonies::put(&Ceremony {
        id: expiring_id.clone(),
        kind: "login".into(),
        user_id: Some(user_id.clone()),
        metadata: Vec::new(),
        state: b"expires".to_vec(),
        expires_at_ms,
    })?;
    thread::sleep(Duration::from_millis(EXPIRY_TEST_MS + 50));
    if lookup::find(&expiring_cookie)?.is_some()
        || ceremonies::take(&expiring_id, "login")?.is_some()
    {
        return Err("expired identity state remained active".into());
    }

    let id = ceremony_id(fixture);
    if ceremonies::take(&id, "login")?.is_some() {
        return Err("wrong ceremony kind matched".into());
    }
    let ceremony = ceremonies::take(&id, "registration")?
        .ok_or_else(|| "registration ceremony was not restored".to_owned())?;
    if ceremony.user_id.as_deref() != Some(&user_id)
        || ceremony.metadata != b"fixture-metadata"
        || ceremony.state != b"fixture-state"
    {
        return Err("restored ceremony is invalid".into());
    }
    if ceremonies::take(&id, "registration")?.is_some() {
        return Err("single-use ceremony was returned twice".into());
    }
    let revoke_cookie = format!("rc_session={token}; rc_session=invalid; rc_session={token}");
    if !management::revoke(&revoke_cookie)? || lookup::find(&cookie)?.is_some() {
        return Err("browser session revocation failed".into());
    }
    if management::revoke(&cookie)? || lookup::find("rc_session=invalid")?.is_some() {
        return Err("missing browser session was treated as active".into());
    }
    println!("identity state: ok");
    Ok(0)
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

fn validate_fixture(value: &str) -> Result<(), String> {
    if !value.is_empty()
        && value.len() <= 48
        && value
            .bytes()
            .all(|byte| byte.is_ascii_lowercase() || byte.is_ascii_digit() || byte == b'-')
    {
        Ok(())
    } else {
        Err("invalid fixture id".into())
    }
}

fn user_id(fixture: &str) -> String {
    format!("fixture-user-{fixture}")
}

fn ceremony_id(fixture: &str) -> String {
    format!("fixture-ceremony-{fixture}")
}

fn now_ms() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_or(0, |duration| duration.as_millis() as u64)
}

export!(IdentityFixture);
