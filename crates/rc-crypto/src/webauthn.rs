use base64::{
    Engine as _,
    engine::general_purpose::{STANDARD, URL_SAFE_NO_PAD},
};
use serde::Deserialize;
use sha2::{Digest, Sha256};
use webauthn_rs_core::proto::{COSEKey, PublicKeyCredential};

#[derive(Debug, thiserror::Error)]
pub enum WebauthnProofError {
    #[error("invalid passkey assertion")]
    Assertion,
    #[error("passkey credential mismatch")]
    CredentialMismatch,
    #[error("invalid stored passkey")]
    StoredPasskey,
    #[error("passkey grant verification failed")]
    Verification,
}

pub fn control_grant_challenge(grant: &str) -> String {
    URL_SAFE_NO_PAD.encode(Sha256::digest(
        format!("rc-control-grant-v1\n{grant}").as_bytes(),
    ))
}

pub fn verify_webauthn_assertion(
    assertion_json: &str,
    credential_id: &str,
    stored_public_key: &str,
    challenge: &str,
    origin: &str,
    rp_id: &str,
) -> Result<(), WebauthnProofError> {
    let assertion: PublicKeyCredential =
        serde_json::from_str(assertion_json).map_err(|_| WebauthnProofError::Assertion)?;
    if assertion.type_ != "public-key"
        || URL_SAFE_NO_PAD.encode(assertion.raw_id.as_ref()) != credential_id
    {
        return Err(WebauthnProofError::CredentialMismatch);
    }
    let client_data: ClientData =
        serde_json::from_slice(assertion.response.client_data_json.as_ref())
            .map_err(|_| WebauthnProofError::Assertion)?;
    if client_data.type_ != "webauthn.get"
        || client_data.challenge != challenge
        || !same_origin(&client_data.origin, origin)
        || client_data.cross_origin
    {
        return Err(WebauthnProofError::Verification);
    }
    let auth_data = assertion.response.authenticator_data.as_ref();
    if auth_data.len() < 37 || auth_data[..32] != Sha256::digest(rp_id.as_bytes())[..] {
        return Err(WebauthnProofError::Verification);
    }
    let flags = auth_data[32];
    if flags & 0x01 == 0 || flags & 0x04 == 0 || (flags & 0x10 != 0 && flags & 0x08 == 0) {
        return Err(WebauthnProofError::Verification);
    }
    let key_bytes = STANDARD
        .decode(stored_public_key)
        .map_err(|_| WebauthnProofError::StoredPasskey)?;
    let key: COSEKey =
        serde_json::from_slice(&key_bytes).map_err(|_| WebauthnProofError::StoredPasskey)?;
    let client_hash = Sha256::digest(assertion.response.client_data_json.as_ref());
    let mut signed = Vec::with_capacity(auth_data.len() + client_hash.len());
    signed.extend_from_slice(auth_data);
    signed.extend_from_slice(&client_hash);
    if !key
        .verify_signature(assertion.response.signature.as_ref(), &signed)
        .map_err(|_| WebauthnProofError::Verification)?
    {
        return Err(WebauthnProofError::Verification);
    }
    Ok(())
}

fn same_origin(left: &str, right: &str) -> bool {
    match (url::Url::parse(left), url::Url::parse(right)) {
        (Ok(left), Ok(right)) => left.origin() == right.origin(),
        _ => false,
    }
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct ClientData {
    #[serde(rename = "type")]
    type_: String,
    challenge: String,
    origin: String,
    #[serde(default)]
    cross_origin: bool,
}
