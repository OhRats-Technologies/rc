use base64::{
    Engine as _,
    engine::general_purpose::{STANDARD, URL_SAFE_NO_PAD},
};
use ring::signature;
use serde::Deserialize;
use sha2::{Digest, Sha256};

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
    let assertion: Assertion =
        serde_json::from_str(assertion_json).map_err(|_| WebauthnProofError::Assertion)?;
    if assertion.type_ != "public-key"
        || URL_SAFE_NO_PAD.encode(decode_assertion(&assertion.raw_id)?) != credential_id
    {
        return Err(WebauthnProofError::CredentialMismatch);
    }
    let client_data_json = decode_assertion(&assertion.response.client_data_json)?;
    let client_data: ClientData =
        serde_json::from_slice(&client_data_json).map_err(|_| WebauthnProofError::Assertion)?;
    if client_data.type_ != "webauthn.get"
        || client_data.challenge != challenge
        || !same_origin(&client_data.origin, origin)
        || client_data.cross_origin
    {
        return Err(WebauthnProofError::Verification);
    }
    let auth_data = decode_assertion(&assertion.response.authenticator_data)?;
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
    let key: StoredCoseKey =
        serde_json::from_slice(&key_bytes).map_err(|_| WebauthnProofError::StoredPasskey)?;
    let client_hash = Sha256::digest(&client_data_json);
    let mut signed = Vec::with_capacity(auth_data.len() + client_hash.len());
    signed.extend_from_slice(&auth_data);
    signed.extend_from_slice(&client_hash);
    let assertion_signature = decode_assertion(&assertion.response.signature)?;
    verify_stored_key(&key, &signed, &assertion_signature)
}

fn decode_assertion(value: &str) -> Result<Vec<u8>, WebauthnProofError> {
    URL_SAFE_NO_PAD
        .decode(value)
        .map_err(|_| WebauthnProofError::Assertion)
}

fn decode_stored(value: &str) -> Result<Vec<u8>, WebauthnProofError> {
    URL_SAFE_NO_PAD
        .decode(value)
        .map_err(|_| WebauthnProofError::StoredPasskey)
}

fn verify_stored_key(
    key: &StoredCoseKey,
    message: &[u8],
    assertion_signature: &[u8],
) -> Result<(), WebauthnProofError> {
    let valid = match (&*key.type_, &key.key) {
        ("ES256", StoredCoseKeyType::EcEc2(ec)) if ec.curve == "SECP256R1" => {
            let x = decode_stored(&ec.x)?;
            let y = decode_stored(&ec.y)?;
            if x.len() != 32 || y.len() != 32 {
                return Err(WebauthnProofError::StoredPasskey);
            }
            let mut public_key = Vec::with_capacity(65);
            public_key.push(0x04);
            public_key.extend_from_slice(&x);
            public_key.extend_from_slice(&y);
            signature::UnparsedPublicKey::new(&signature::ECDSA_P256_SHA256_ASN1, public_key)
                .verify(message, assertion_signature)
                .is_ok()
        }
        ("RS256", StoredCoseKeyType::Rsa(rsa)) => {
            let n = decode_stored(&rsa.n)?;
            let components = signature::RsaPublicKeyComponents {
                n: n.as_slice(),
                e: rsa.e.as_slice(),
            };
            components
                .verify(
                    &signature::RSA_PKCS1_2048_8192_SHA256,
                    message,
                    assertion_signature,
                )
                .is_ok()
        }
        _ => return Err(WebauthnProofError::StoredPasskey),
    };
    valid.then_some(()).ok_or(WebauthnProofError::Verification)
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

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct Assertion {
    #[serde(rename = "type")]
    type_: String,
    raw_id: String,
    response: AssertionResponse,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct AssertionResponse {
    #[serde(rename = "clientDataJSON")]
    client_data_json: String,
    authenticator_data: String,
    signature: String,
}

#[derive(Deserialize)]
struct StoredCoseKey {
    type_: String,
    key: StoredCoseKeyType,
}

#[derive(Deserialize)]
enum StoredCoseKeyType {
    #[serde(rename = "EC_EC2")]
    EcEc2(StoredEc2Key),
    #[serde(rename = "RSA")]
    Rsa(StoredRsaKey),
}

#[derive(Deserialize)]
struct StoredEc2Key {
    curve: String,
    x: String,
    y: String,
}

#[derive(Deserialize)]
struct StoredRsaKey {
    n: String,
    e: [u8; 3],
}
