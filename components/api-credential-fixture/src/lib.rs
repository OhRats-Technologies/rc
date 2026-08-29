wit_bindgen::generate!({ path: "../../wit", world: "api-credential-fixture", generate_all });

mod admin;
mod proof;

use ohrats::{
    rc_api_credentials::{
        credentials,
        types::{Kind, Lifetime, Scope},
    },
    rc_plugin::types::{Command, Requirement, Selection},
};

struct ApiCredentialFixture;

impl Guest for ApiCredentialFixture {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:api-credential-fixture".into(),
            version: "0.1.0".into(),
            provides: Vec::new(),
            requires: vec![
                requirement("ohrats:rc-api-credentials/credentials"),
                requirement("ohrats:rc-identity/admin-issuer"),
                requirement("ohrats:rc-identity/credentials"),
                requirement("ohrats:rc-session/management"),
            ],
            commands: vec![
                command(
                    "api-credentials-seed",
                    "Seed API and CLI credential state",
                    "rc api-credentials-seed <fixture>",
                ),
                command(
                    "api-credentials-verify",
                    "Verify API and CLI credential state",
                    "rc api-credentials-verify <fixture>",
                ),
                command(
                    "api-credentials-race",
                    "Race one API nonce",
                    "rc api-credentials-race <fixture>",
                ),
            ],
        }
    }
    fn activate() -> Result<(), String> {
        Ok(())
    }
    fn deactivate() {}
    fn invoke(command: String, args: Vec<String>) -> Result<u32, String> {
        let [fixture] = args.as_slice() else {
            return Err("usage: rc api-credentials-<seed|verify|race> <fixture>".into());
        };
        valid(fixture)?;
        match command.as_str() {
            "api-credentials-seed" => seed(fixture),
            "api-credentials-verify" => verify(fixture),
            "api-credentials-race" => race(fixture),
            _ => Err(format!("unsupported command {command:?}")),
        }
    }
}

fn seed(fixture: &str) -> Result<u32, String> {
    admin::create_user(fixture)?;
    let api_id = format!("api-{fixture}");
    let api_key = proof::key(7);
    if credentials::create_api(
        &admin::forged_token(),
        &format!("forged-{fixture}"),
        "Forged token",
        &api_key,
        &[Scope::Read],
        None,
    )
    .is_ok()
    {
        return Err("forged admin token was accepted".into());
    }
    let wrong_operation = admin::authorization(admin::BROWSER, fixture, "api-credential.revoke")?;
    if credentials::create_api(
        &wrong_operation,
        &format!("wrong-{fixture}"),
        "Wrong operation",
        &api_key,
        &[Scope::Read],
        None,
    )
    .is_ok()
    {
        return Err("operation-mismatched admin proof was accepted".into());
    }
    let create = admin::authorization(admin::BROWSER, fixture, "api-credential.create")?;
    credentials::create_api(
        &create,
        &api_id,
        "Fixture API",
        &api_key,
        &[Scope::Read, Scope::Execute],
        None,
    )?;
    if credentials::create_api(
        &create,
        &format!("replay-{fixture}"),
        "Replayed proof",
        &api_key,
        &[Scope::Read],
        None,
    )
    .is_ok()
    {
        return Err("administrator proof was reusable".into());
    }
    let expiry_id = format!("exp-{fixture}");
    credentials::create_api(
        &admin::authorization(admin::BROWSER, fixture, "api-credential.create")?,
        &expiry_id,
        "Expires",
        &api_key,
        &[Scope::Read],
        Some(Lifetime::OneHour),
    )?;
    let cli_id = format!("cli-{fixture}");
    let cli_key = proof::key(9);
    let request_id = format!("req-{fixture}");
    let device_code = format!("device-{fixture}");
    let user_code = format!("user-{fixture}");
    let started_at = proof::now_ms();
    let start = credentials::start_cli(
        &cli_id,
        &cli_key,
        Some(Lifetime::OneDay),
        &request_id,
        &device_code,
        &user_code,
        started_at,
    )?;
    if credentials::poll_cli(&start.request_id, &device_code, started_at)?.is_some() {
        return Err("unapproved CLI authorization was issued".into());
    }
    if admin::forged_authorization(admin::BROWSER, fixture).is_ok() {
        return Err("forged admin proof was issued".into());
    }
    let wrong_client = admin::authorization(admin::BROWSER, fixture, "api-credential.cli-approve")?;
    if credentials::approve_cli(&wrong_client, &start.request_id, &user_code, &cli_key).is_ok() {
        return Err("client-mismatched admin proof was accepted".into());
    }
    credentials::approve_cli(
        &admin::authorization(&cli_id, fixture, "api-credential.cli-approve")?,
        &start.request_id,
        &user_code,
        &cli_key,
    )?;
    println!("api credential seed: ok");
    Ok(0)
}

fn verify(fixture: &str) -> Result<u32, String> {
    let api = format!("api-{fixture}");
    let expiring = format!("exp-{fixture}");
    let cli = format!("cli-{fixture}");
    let values = credentials::all(admin::USER)?;
    if values.len() != 3 || values.iter().filter(|v| v.kind == Kind::Api).count() != 2 {
        return Err("credential persistence failed".into());
    }
    let valid_at = proof::now_ms();
    let valid_request = proof::signed(
        7,
        &api,
        valid_at,
        "nonce-fixture-0001",
        "GET",
        "/api/v1/me",
        b"",
    );
    let verified = credentials::verify(&valid_request, valid_at)?;
    if verified.credential_id != api || !verified.scopes.contains(&Scope::Execute) {
        return Err("API proof or scopes failed".into());
    }
    if credentials::verify(&valid_request, valid_at).is_ok() {
        return Err("successful nonce was replayable".into());
    }
    let changed_at = proof::now_ms();
    let mut changed_request = proof::signed(
        7,
        &api,
        changed_at,
        "nonce-fixture-0002",
        "POST",
        "/api/v1/me",
        b"original",
    );
    changed_request.body = b"changed".to_vec();
    if credentials::verify(&changed_request, changed_at).is_ok() {
        return Err("body-bound signature was accepted".into());
    }
    let polled_at = proof::now_ms();
    let cli_value = credentials::poll_cli(
        &format!("req-{fixture}"),
        &format!("device-{fixture}"),
        polled_at,
    )?
    .ok_or("approved CLI was not returned")?;
    if cli_value.id != cli
        || credentials::poll_cli(
            &format!("req-{fixture}"),
            &format!("device-{fixture}"),
            polled_at,
        )
        .is_ok()
    {
        return Err("CLI authorization was reusable".into());
    }
    let cli_at = proof::now_ms();
    let cli_proof = proof::signed(
        9,
        &cli,
        cli_at,
        "nonce-cli-000001",
        "GET",
        "/api/v1/devices",
        b"",
    );
    credentials::verify(&cli_proof, cli_at)?;
    let expires_at = credentials::get(&expiring)?
        .ok_or_else(|| "expiring credential was not restored".to_owned())?
        .expires_at_ms;
    let expired_at = expires_at.saturating_add(1_000);
    let expired_request = proof::signed(
        7,
        &expiring,
        expired_at,
        "nonce-expiry-0001",
        "GET",
        "/",
        b"",
    );
    if credentials::verify(&expired_request, expired_at).is_ok() {
        return Err("expired credential was accepted".into());
    }
    let revoked_at = proof::now_ms();
    let revoked_request = proof::signed(7, &api, revoked_at, "nonce-revoked-01", "GET", "/", b"");
    if !credentials::revoke(
        &admin::authorization(admin::BROWSER, fixture, "api-credential.revoke")?,
        &api,
    )? || credentials::verify(&revoked_request, revoked_at).is_ok()
    {
        return Err("revoked credential remained valid".into());
    }
    println!("api credential state: ok");
    Ok(0)
}

fn race(fixture: &str) -> Result<u32, String> {
    let id = format!("api-{fixture}");
    let now = proof::now_ms();
    let request = proof::signed(7, &id, now, "nonce-race-00001", "GET", "/", b"");
    credentials::verify(&request, now)?;
    Ok(0)
}
fn valid(value: &str) -> Result<(), String> {
    if !value.is_empty()
        && value.len() <= 40
        && value
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'-')
    {
        Ok(())
    } else {
        Err("invalid fixture".into())
    }
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

export!(ApiCredentialFixture);
