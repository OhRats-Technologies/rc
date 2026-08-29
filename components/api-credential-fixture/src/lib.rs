wit_bindgen::generate!({ path: "../../wit", world: "api-credential-fixture", generate_all });

use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use ed25519_dalek::{Signer, SigningKey};
use ohrats::{
    rc_api_credentials::{
        credentials,
        types::{Administrator, Kind, Lifetime, Request, Scope},
    },
    rc_plugin::types::{Command, Requirement},
};
use sha2::{Digest, Sha256};

const NOW: u64 = 1_700_000_000_000;
const USER: &str = "fixture-user";
const BROWSER: &str = "browser-fixture";

struct ApiCredentialFixture;

impl Guest for ApiCredentialFixture {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:api-credential-fixture".into(),
            version: "0.1.0".into(),
            provides: Vec::new(),
            requires: vec![Requirement {
                name: "ohrats:rc-api-credentials/credentials".into(),
                version: "^0.1".into(),
                selection: ohrats::rc_plugin::types::Selection::Single,
            }],
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
    let admin_value = admin(NOW, BROWSER);
    let api_id = format!("api-{fixture}");
    let api_key = key(7);
    credentials::create_api(
        &admin_value,
        &api_id,
        "Fixture API",
        &api_key,
        &[Scope::Read, Scope::Execute],
        None,
    )?;
    let expiry_id = format!("exp-{fixture}");
    credentials::create_api(
        &admin_value,
        &expiry_id,
        "Expires",
        &api_key,
        &[Scope::Read],
        Some(Lifetime::OneHour),
    )?;
    let cli_id = format!("cli-{fixture}");
    let cli_key = key(9);
    let request_id = format!("req-{fixture}");
    let device_code = format!("device-{fixture}");
    let user_code = format!("user-{fixture}");
    let start = credentials::start_cli(
        &cli_id,
        &cli_key,
        Some(Lifetime::OneDay),
        &request_id,
        &device_code,
        &user_code,
        NOW,
    )?;
    if credentials::poll_cli(&start.request_id, &device_code, NOW)?.is_some() {
        return Err("unapproved CLI authorization was issued".into());
    }
    let stale = Administrator {
        user_id: USER.into(),
        browser_client_id: cli_id.clone(),
        passkey_step_up_at_ms: NOW - 121_000,
        now_ms: NOW,
    };
    if credentials::approve_cli(&stale, &start.request_id, &user_code, &cli_key).is_ok() {
        return Err("stale admin proof was accepted".into());
    }
    credentials::approve_cli(
        &admin(NOW, &cli_id),
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
    let values = credentials::all(USER)?;
    if values.len() != 3 || values.iter().filter(|v| v.kind == Kind::Api).count() != 2 {
        return Err("credential persistence failed".into());
    }
    let valid_request = signed(
        7,
        &api,
        "1700000000",
        "nonce-fixture-0001",
        "GET",
        "/api/v1/me",
        b"",
    );
    let verified = credentials::verify(&valid_request, NOW + 1_000)?;
    if verified.credential_id != api || !verified.scopes.contains(&Scope::Execute) {
        return Err("API proof or scopes failed".into());
    }
    if credentials::verify(&valid_request, NOW + 1_000).is_ok() {
        return Err("successful nonce was replayable".into());
    }
    let mut changed_request = signed(
        7,
        &api,
        "1700000000",
        "nonce-fixture-0002",
        "POST",
        "/api/v1/me",
        b"original",
    );
    changed_request.body = b"changed".to_vec();
    if credentials::verify(&changed_request, NOW + 1_000).is_ok() {
        return Err("body-bound signature was accepted".into());
    }
    let cli_value = credentials::poll_cli(
        &format!("req-{fixture}"),
        &format!("device-{fixture}"),
        NOW + 1_000,
    )?
    .ok_or("approved CLI was not returned")?;
    if cli_value.id != cli
        || credentials::poll_cli(
            &format!("req-{fixture}"),
            &format!("device-{fixture}"),
            NOW + 1_000,
        )
        .is_ok()
    {
        return Err("CLI authorization was reusable".into());
    }
    let cli_proof = signed(
        9,
        &cli,
        "1700000000",
        "nonce-cli-000001",
        "GET",
        "/api/v1/devices",
        b"",
    );
    credentials::verify(&cli_proof, NOW + 1_000)?;
    let expired_request = signed(
        7,
        &expiring,
        "1700003601",
        "nonce-expiry-0001",
        "GET",
        "/",
        b"",
    );
    if credentials::verify(&expired_request, NOW + 3_601_000).is_ok() {
        return Err("expired credential was accepted".into());
    }
    let revoked_request = signed(7, &api, "1700000002", "nonce-revoked-01", "GET", "/", b"");
    if !credentials::revoke(&admin(NOW + 2_000, BROWSER), &api)?
        || credentials::verify(&revoked_request, NOW + 2_000).is_ok()
    {
        return Err("revoked credential remained valid".into());
    }
    println!("api credential state: ok");
    Ok(0)
}

fn race(fixture: &str) -> Result<u32, String> {
    let id = format!("api-{fixture}");
    let request = signed(7, &id, "1700000001", "nonce-race-00001", "GET", "/", b"");
    credentials::verify(&request, NOW + 1_000)?;
    Ok(0)
}

fn signed(
    seed: u8,
    id: &str,
    timestamp: &str,
    nonce: &str,
    method: &str,
    path: &str,
    body: &[u8],
) -> Request {
    let body_hash = Sha256::digest(body);
    let digest: String = body_hash.iter().map(|b| format!("{b:02x}")).collect();
    let payload = format!("rc-api-v1\n{id}\n{timestamp}\n{nonce}\n{method}\n{path}\n{digest}");
    let signature = SigningKey::from_bytes(&[seed; 32]).sign(payload.as_bytes());
    Request {
        key_id: id.into(),
        timestamp_seconds: timestamp.into(),
        nonce: nonce.into(),
        method: method.into(),
        path_and_raw_query: path.into(),
        body: body.to_vec(),
        signature: URL_SAFE_NO_PAD.encode(signature.to_bytes()),
    }
}

fn key(seed: u8) -> String {
    URL_SAFE_NO_PAD.encode(
        SigningKey::from_bytes(&[seed; 32])
            .verifying_key()
            .to_bytes(),
    )
}
fn admin(now_ms: u64, browser_client_id: &str) -> Administrator {
    Administrator {
        user_id: USER.into(),
        browser_client_id: browser_client_id.into(),
        passkey_step_up_at_ms: now_ms.saturating_sub(1_000),
        now_ms,
    }
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
fn command(name: &str, summary: &str, usage: &str) -> Command {
    Command {
        name: name.into(),
        summary: summary.into(),
        usage: usage.into(),
    }
}

export!(ApiCredentialFixture);
