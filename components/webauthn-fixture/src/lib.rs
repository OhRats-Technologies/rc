wit_bindgen::generate!({
    path: "../../wit",
    world: "webauthn-fixture",
    generate_all,
});

mod fixture;

use ohrats::{
    rc_plugin::types::{Command, Requirement, Selection},
    rc_webauthn::verifier,
};

struct WebauthnFixture;

impl Guest for WebauthnFixture {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:webauthn-fixture".into(),
            version: "0.1.0".into(),
            provides: Vec::new(),
            requires: vec![Requirement {
                name: "ohrats:rc-webauthn/verifier".into(),
                version: "^0.1".into(),
                selection: Selection::Keyed,
            }],
            commands: vec![Command {
                name: "webauthn-check".into(),
                summary: "Verify deterministic ES256 registration and authentication".into(),
                usage: "rc webauthn-check".into(),
            }],
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }

    fn deactivate() {}

    fn invoke(command: String, args: Vec<String>) -> Result<u32, String> {
        if command != "webauthn-check" || !args.is_empty() {
            return Err("usage: rc webauthn-check".into());
        }
        check()?;
        println!("webauthn verifier: ok");
        Ok(0)
    }
}

fn check() -> Result<(), String> {
    let value = fixture::load()?;
    let registration =
        verifier::verify_registration(&value.algorithm, &value.registration_request()?)?;
    if registration.credential.algorithm != "es256"
        || registration.credential.sign_count != 0
        || registration.aaguid != vec![1; 16]
        || !registration.user_verified
    {
        return Err("registration verifier returned invalid credential state".into());
    }
    let authentication = verifier::verify_authentication(
        &value.algorithm,
        &value.authentication_request(registration.credential.clone())?,
    )?;
    if authentication.credential.sign_count != 1
        || !authentication.sign_count_advanced
        || !authentication.user_verified
    {
        return Err("authentication verifier returned invalid credential state".into());
    }
    let mut tampered = value.authentication_request(registration.credential)?;
    let Some(first) = tampered.signature.first_mut() else {
        return Err("fixture signature is empty".into());
    };
    *first ^= 0x01;
    if verifier::verify_authentication(&value.algorithm, &tampered).is_ok() {
        return Err("tampered authentication unexpectedly verified".into());
    }
    Ok(())
}

export!(WebauthnFixture);
