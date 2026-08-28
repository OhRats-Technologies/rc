use crate::ohrats::rc_webauthn::types::{
    AuthenticationRequest, RegistrationRequest, RelyingParty, StoredCredential, UserHandleMode,
    UserVerification,
};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use serde::Deserialize;

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Fixture {
    pub algorithm: String,
    rp_id: String,
    origin: String,
    expected_user_handle: String,
    registration: Registration,
    authentication: Authentication,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct Registration {
    challenge: String,
    credential_id: String,
    #[serde(rename = "clientDataJSON")]
    client_data_json: String,
    attestation_object: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct Authentication {
    challenge: String,
    credential_id: String,
    #[serde(rename = "clientDataJSON")]
    client_data_json: String,
    authenticator_data: String,
    signature: String,
    user_handle: String,
}

pub fn load() -> Fixture {
    serde_json::from_str(include_str!("../../../../fixtures/webauthn-es256.json"))
        .expect("WebAuthn fixture should parse")
}

impl Fixture {
    pub fn registration_request(&self) -> RegistrationRequest {
        RegistrationRequest {
            relying_party: self.relying_party(),
            challenge: decode(&self.registration.challenge),
            credential_id: decode(&self.registration.credential_id),
            client_data_json: decode(&self.registration.client_data_json),
            attestation_object: decode(&self.registration.attestation_object),
            user_verification: UserVerification::Required,
        }
    }

    pub fn authentication_request(&self, credential: StoredCredential) -> AuthenticationRequest {
        AuthenticationRequest {
            relying_party: self.relying_party(),
            challenge: decode(&self.authentication.challenge),
            credential_id: decode(&self.authentication.credential_id),
            client_data_json: decode(&self.authentication.client_data_json),
            authenticator_data: decode(&self.authentication.authenticator_data),
            signature: decode(&self.authentication.signature),
            credential,
            expected_user_handle: decode(&self.expected_user_handle),
            response_user_handle: Some(decode(&self.authentication.user_handle)),
            user_handle_mode: UserHandleMode::Identified,
            user_verification: UserVerification::Required,
        }
    }

    fn relying_party(&self) -> RelyingParty {
        RelyingParty {
            id: self.rp_id.clone(),
            origin: self.origin.clone(),
        }
    }
}

fn decode(value: &str) -> Vec<u8> {
    URL_SAFE_NO_PAD
        .decode(value)
        .expect("WebAuthn fixture field should decode")
}
