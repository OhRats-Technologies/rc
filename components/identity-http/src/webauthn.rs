use crate::{
    config::RelyingParty,
    ohrats::{
        rc_identity::{ceremonies, credentials, types::Ceremony},
        rc_webauthn::{
            types::{
                AuthenticationRequest, RegistrationRequest, RelyingParty as WireRelyingParty,
                UserHandleMode, UserVerification,
            },
            verifier,
        },
    },
    time,
};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use serde::{Deserialize, Serialize};
use serde_json::Value;

const CEREMONY_TTL_MS: u64 = 5 * 60 * 1000;
const MAX_CREDENTIALS: usize = 1000;

pub struct RegistrationResult {
    pub user_id: String,
    pub display_name: String,
    pub credential: crate::ohrats::rc_webauthn::types::StoredCredential,
}

#[derive(Serialize, Deserialize)]
struct RegistrationMeta {
    display_name: String,
}

#[derive(Clone, Serialize, Deserialize)]
struct CeremonyState {
    challenge: String,
    rp_id: String,
    origin: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct CredentialResponse<T> {
    #[serde(rename = "rawId")]
    raw_id: String,
    #[allow(dead_code)]
    id: String,
    #[allow(dead_code)]
    r#type: String,
    response: T,
}

#[derive(Deserialize)]
struct RegistrationResponse {
    #[serde(rename = "clientDataJSON")]
    client_data_json: String,
    #[serde(rename = "attestationObject")]
    attestation_object: String,
}

#[derive(Deserialize)]
struct AuthenticationResponse {
    #[serde(rename = "clientDataJSON")]
    client_data_json: String,
    #[serde(rename = "authenticatorData")]
    authenticator_data: String,
    signature: String,
    #[serde(rename = "userHandle")]
    user_handle: Option<String>,
}

pub fn begin_registration(
    kind: &str,
    user_id: &str,
    display_name: &str,
    relying_party: &RelyingParty,
) -> Result<(String, Value), String> {
    let challenge = random(32)?;
    let ceremony_id = random_id()?;
    let excludes = credentials::all(Some(user_id))?
        .into_iter()
        .map(|value| {
            serde_json::json!({
                "id": URL_SAFE_NO_PAD.encode(value.credential.id),
                "type": "public-key",
            })
        })
        .collect::<Vec<_>>();
    let options = serde_json::json!({
        "challenge": URL_SAFE_NO_PAD.encode(&challenge),
        "rp": { "id": relying_party.id, "name": "RC" },
        "user": {
            "id": URL_SAFE_NO_PAD.encode(user_id.as_bytes()),
            "name": display_name,
            "displayName": display_name,
        },
        "pubKeyCredParams": [{ "type": "public-key", "alg": -7 }],
        "timeout": CEREMONY_TTL_MS,
        "attestation": "none",
        "authenticatorSelection": {
            "residentKey": "required",
            "requireResidentKey": true,
            "userVerification": "required",
        },
        "excludeCredentials": excludes,
    });
    ceremonies::put(&Ceremony {
        id: ceremony_id.clone(),
        kind: kind.into(),
        user_id: Some(user_id.into()),
        metadata: serde_json::to_vec(&RegistrationMeta {
            display_name: display_name.into(),
        })
        .map_err(display)?,
        state: serde_json::to_vec(&CeremonyState {
            challenge: URL_SAFE_NO_PAD.encode(challenge),
            rp_id: relying_party.id.clone(),
            origin: relying_party.origin.clone(),
        })
        .map_err(display)?,
        expires_at_ms: time::now_ms().saturating_add(CEREMONY_TTL_MS),
    })?;
    Ok((ceremony_id, options))
}

pub fn finish_registration(
    kind: &str,
    ceremony_id: &str,
    response: Value,
) -> Result<RegistrationResult, String> {
    let ceremony =
        ceremonies::take(ceremony_id, kind)?.ok_or_else(|| "registration expired".to_owned())?;
    let user_id = ceremony
        .user_id
        .ok_or_else(|| "registration expired".to_owned())?;
    let meta: RegistrationMeta = serde_json::from_slice(&ceremony.metadata).map_err(display)?;
    let state: CeremonyState = serde_json::from_slice(&ceremony.state).map_err(display)?;
    let response: CredentialResponse<RegistrationResponse> =
        serde_json::from_value(response).map_err(|_| "invalid passkey response".to_owned())?;
    let credential_id = decode(&response.raw_id, "credential id")?;
    let challenge = decode(&state.challenge, "challenge")?;
    let verified = verifier::verify_registration(
        "es256",
        &RegistrationRequest {
            relying_party: wire_rp(state),
            challenge,
            credential_id,
            client_data_json: decode(&response.response.client_data_json, "client data")?,
            attestation_object: decode(&response.response.attestation_object, "attestation")?,
            user_verification: UserVerification::Required,
        },
    )?;
    Ok(RegistrationResult {
        user_id,
        display_name: meta.display_name,
        credential: verified.credential,
    })
}

pub fn begin_login(relying_party: &RelyingParty) -> Result<(String, Value), String> {
    let passkeys = credentials::all(None)?;
    if passkeys.is_empty() {
        return Err("No passkey is registered yet. Complete RC setup first.".into());
    }
    if passkeys.len() > MAX_CREDENTIALS {
        return Err("too many passkeys are registered".into());
    }
    let challenge = random(32)?;
    let ceremony_id = random_id()?;
    let options = serde_json::json!({
        "challenge": URL_SAFE_NO_PAD.encode(&challenge),
        "rpId": relying_party.id,
        "timeout": CEREMONY_TTL_MS,
        "userVerification": "required",
        "allowCredentials": passkeys.into_iter().map(|value| serde_json::json!({
            "id": URL_SAFE_NO_PAD.encode(value.credential.id),
            "type": "public-key",
        })).collect::<Vec<_>>(),
    });
    ceremonies::put(&Ceremony {
        id: ceremony_id.clone(),
        kind: "login".into(),
        user_id: None,
        metadata: Vec::new(),
        state: serde_json::to_vec(&CeremonyState {
            challenge: URL_SAFE_NO_PAD.encode(challenge),
            rp_id: relying_party.id.clone(),
            origin: relying_party.origin.clone(),
        })
        .map_err(display)?,
        expires_at_ms: time::now_ms().saturating_add(CEREMONY_TTL_MS),
    })?;
    Ok((ceremony_id, options))
}

pub fn finish_login(ceremony_id: &str, response: Value) -> Result<String, String> {
    let ceremony = ceremonies::take(ceremony_id, "login")?
        .ok_or_else(|| "authentication expired".to_owned())?;
    let state: CeremonyState = serde_json::from_slice(&ceremony.state).map_err(display)?;
    let response: CredentialResponse<AuthenticationResponse> =
        serde_json::from_value(response).map_err(|_| "invalid passkey response".to_owned())?;
    let credential_id = decode(&response.raw_id, "credential id")?;
    let passkey = credentials::get_by_credential_id(&credential_id)?
        .ok_or_else(|| "unknown passkey".to_owned())?;
    let verified = verifier::verify_authentication(
        "es256",
        &AuthenticationRequest {
            relying_party: wire_rp(state.clone()),
            challenge: decode(&state.challenge, "challenge")?,
            credential_id,
            client_data_json: decode(&response.response.client_data_json, "client data")?,
            authenticator_data: decode(
                &response.response.authenticator_data,
                "authenticator data",
            )?,
            signature: decode(&response.response.signature, "signature")?,
            credential: passkey.credential,
            expected_user_handle: passkey.user_id.as_bytes().to_vec(),
            response_user_handle: response
                .response
                .user_handle
                .as_deref()
                .map(|value| decode(value, "user handle"))
                .transpose()?,
            user_handle_mode: UserHandleMode::Identified,
            user_verification: UserVerification::Required,
        },
    )?;
    credentials::update(&passkey.id, &verified.credential, time::now_ms())?;
    Ok(passkey.user_id)
}

fn wire_rp(value: CeremonyState) -> WireRelyingParty {
    WireRelyingParty {
        id: value.rp_id,
        origin: value.origin,
    }
}

fn decode(value: &str, label: &str) -> Result<Vec<u8>, String> {
    URL_SAFE_NO_PAD
        .decode(value)
        .map_err(|_| format!("invalid {label}"))
}

fn random(size: usize) -> Result<Vec<u8>, String> {
    let mut bytes = vec![0; size];
    getrandom::fill(&mut bytes).map_err(display)?;
    Ok(bytes)
}

fn random_id() -> Result<String, String> {
    Ok(URL_SAFE_NO_PAD.encode(random(18)?))
}

fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}
