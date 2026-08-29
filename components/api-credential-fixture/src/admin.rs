use crate::ohrats::{
    rc_identity::{admin_issuer, credentials as identity_credentials, types::HumanAuthorization},
    rc_session::management,
    rc_webauthn::types::{
        AuthenticationRequest, RelyingParty, StoredCredential, UserHandleMode, UserVerification,
    },
};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use p256::{
    FieldBytes, Scalar, U256,
    ecdsa::Signature,
    elliptic_curve::{ff::PrimeField, ops::Reduce},
};
use serde_cbor_2::Value;
use sha2::{Digest, Sha256};
use std::collections::BTreeMap;

pub const USER: &str = "fixture-user";
pub const BROWSER: &str = "browser-fixture";
const RP_ID: &str = "rc.example.test";
const ORIGIN: &str = "https://rc.example.test";
const PRIVATE_KEY: [u8; 32] = [3; 32];
const PUBLIC_X: [u8; 32] = [
    0x59, 0x1a, 0xb7, 0x71, 0xeb, 0xbc, 0xfd, 0x6d, 0x9c, 0xb9, 0x09, 0x4d, 0x10, 0x65, 0x28, 0xad,
    0xd1, 0xa6, 0x9d, 0x44, 0xc2, 0xc1, 0xf6, 0x27, 0xf0, 0x89, 0xec, 0x58, 0xb9, 0xc6, 0x1a, 0xdf,
];
const PUBLIC_Y: [u8; 32] = [
    0x9f, 0x4e, 0x6a, 0xbf, 0x0d, 0x04, 0x5c, 0x0c, 0x69, 0x3a, 0x3c, 0x68, 0xad, 0x7c, 0x97, 0xca,
    0x72, 0xbe, 0x64, 0xde, 0xf4, 0xa2, 0x6f, 0xec, 0xd2, 0x63, 0xdd, 0x98, 0xa9, 0x27, 0x80, 0xf0,
];
// k=1 is deliberately fixture-only. Its point is the P-256 generator, so r is
// known and signing needs only scalar arithmetic instead of a fuel-heavy point
// multiplication inside the constrained guest command.
const FIXTURE_R: [u8; 32] = [
    0x6b, 0x17, 0xd1, 0xf2, 0xe1, 0x2c, 0x42, 0x47, 0xf8, 0xbc, 0xe6, 0xe5, 0x63, 0xa4, 0x40, 0xf2,
    0x77, 0x03, 0x7d, 0x81, 0x2d, 0xeb, 0x33, 0xa0, 0xf4, 0xa1, 0x39, 0x45, 0xd8, 0x98, 0xc2, 0x96,
];

pub fn create_user(fixture: &str) -> Result<(), String> {
    identity_credentials::create_user(
        USER,
        "Fixture User",
        "Fixture Passkey",
        &fixture_credential(fixture, 0),
    )?;
    Ok(())
}

pub fn authorization(
    browser_client_id: &str,
    fixture: &str,
    operation: &str,
) -> Result<HumanAuthorization, String> {
    let issued = management::issue(USER, 0)?;
    let cookie = format!("rc_session={}", issued.token);
    let challenge = admin_issuer::begin(&cookie, browser_client_id, operation, &relying_party())?;
    let authentication = authentication(&challenge, fixture)?;
    admin_issuer::issue(&cookie, browser_client_id, &challenge.id, &authentication)
}

pub fn forged_authorization(
    browser_client_id: &str,
    fixture: &str,
) -> Result<HumanAuthorization, String> {
    let issued = management::issue(USER, 0)?;
    let cookie = format!("rc_session={}", issued.token);
    let challenge = admin_issuer::begin(
        &cookie,
        browser_client_id,
        "api-credential.create",
        &relying_party(),
    )?;
    let mut request = authentication(&challenge, fixture)?;
    request.signature[0] ^= 1;
    admin_issuer::issue(&cookie, browser_client_id, &challenge.id, &request)
}

pub fn forged_token() -> HumanAuthorization {
    HumanAuthorization { token: vec![0; 32] }
}

fn authentication(
    challenge: &admin_issuer::Challenge,
    fixture: &str,
) -> Result<AuthenticationRequest, String> {
    let credential_id = format!("credential-{fixture}").into_bytes();
    let credential = identity_credentials::get_by_credential_id(&credential_id)?
        .ok_or_else(|| "fixture passkey is missing".to_owned())?
        .credential;
    let sign_count = credential
        .sign_count
        .checked_add(1)
        .ok_or_else(|| "fixture passkey sign count exhausted".to_owned())?;
    let challenge_b64 = URL_SAFE_NO_PAD.encode(&challenge.challenge);
    let client_data = format!(
        "{{\"type\":\"webauthn.get\",\"challenge\":\"{challenge_b64}\",\"origin\":\"{ORIGIN}\",\"crossOrigin\":false}}"
    )
    .into_bytes();
    let mut authenticator_data = Sha256::digest(RP_ID.as_bytes()).to_vec();
    authenticator_data.push(0x0d);
    authenticator_data.extend(sign_count.to_be_bytes());
    let mut signed = authenticator_data.clone();
    signed.extend(Sha256::digest(&client_data));
    let signature = fixture_signature(&signed)?;
    Ok(AuthenticationRequest {
        relying_party: challenge.relying_party.clone(),
        challenge: challenge.challenge.clone(),
        credential_id: credential.id.clone(),
        client_data_json: client_data,
        authenticator_data,
        signature: signature.to_der().as_bytes().to_vec(),
        credential,
        expected_user_handle: USER.as_bytes().to_vec(),
        response_user_handle: Some(USER.as_bytes().to_vec()),
        user_handle_mode: UserHandleMode::Identified,
        user_verification: UserVerification::Required,
    })
}

fn fixture_credential(fixture: &str, sign_count: u32) -> StoredCredential {
    let mut map = BTreeMap::new();
    map.insert(Value::Integer(1), Value::Integer(2));
    map.insert(Value::Integer(3), Value::Integer(-7));
    map.insert(Value::Integer(-1), Value::Integer(1));
    map.insert(Value::Integer(-2), Value::Bytes(PUBLIC_X.to_vec()));
    map.insert(Value::Integer(-3), Value::Bytes(PUBLIC_Y.to_vec()));
    StoredCredential {
        id: format!("credential-{fixture}").into_bytes(),
        algorithm: "es256".into(),
        public_key_cose: serde_cbor_2::to_vec(&Value::Map(map)).expect("fixture COSE key"),
        sign_count,
        backup_eligible: true,
        backup_state: false,
    }
}

fn relying_party() -> RelyingParty {
    RelyingParty {
        id: RP_ID.into(),
        origin: ORIGIN.into(),
    }
}

fn fixture_signature(message: &[u8]) -> Result<Signature, String> {
    let digest: FieldBytes = Sha256::digest(message);
    let z = <Scalar as Reduce<U256>>::reduce_bytes(&digest);
    let private = scalar(PRIVATE_KEY, "fixture private key")?;
    let r = scalar(FIXTURE_R, "fixture nonce point")?;
    let s = z.add(&r.multiply(&private));
    Signature::from_scalars(r.to_bytes(), s.to_bytes()).map_err(display)
}

fn scalar(bytes: [u8; 32], label: &str) -> Result<Scalar, String> {
    Option::<Scalar>::from(Scalar::from_repr(bytes.into()))
        .ok_or_else(|| format!("invalid {label}"))
}

fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}
