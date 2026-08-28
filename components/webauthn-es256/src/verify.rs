use crate::ohrats::rc_webauthn::types::{
    AuthenticationRequest, RegistrationRequest, StoredCredential as WireCredential, UserHandleMode,
    UserVerification, VerifiedAuthentication, VerifiedRegistration,
};
use passkey_rp::{
    AuthenticationVerification, Challenge, CredentialBackupState, PasskeyRp, PublicKey,
    RegistrationVerification, RelyingParty, StoredCredential, UserHandleVerification,
    UserVerificationPolicy,
};

const ALGORITHM: &str = "es256";
const MAX_USER_HANDLE_BYTES: usize = 64;

pub fn registration(
    algorithm: &str,
    value: RegistrationRequest,
) -> Result<VerifiedRegistration, String> {
    require_algorithm(algorithm)?;
    let RegistrationRequest {
        relying_party,
        challenge,
        credential_id,
        client_data_json,
        attestation_object,
        user_verification,
    } = value;
    let verifier = verifier(relying_party.id, relying_party.origin)?;
    let challenge = Challenge::from_bytes(challenge).map_err(display)?;
    let request = RegistrationVerification::new(
        &challenge,
        &credential_id,
        &client_data_json,
        &attestation_object,
    )
    .with_user_verification(policy(user_verification));
    let verified = verifier.verify_registration(request).map_err(display)?;
    Ok(VerifiedRegistration {
        credential: encode_credential(verified.credential())?,
        aaguid: verified.aaguid().to_vec(),
        user_verified: verified.user_verified(),
    })
}

pub fn authentication(
    algorithm: &str,
    value: AuthenticationRequest,
) -> Result<VerifiedAuthentication, String> {
    require_algorithm(algorithm)?;
    let AuthenticationRequest {
        relying_party,
        challenge,
        credential_id,
        client_data_json,
        authenticator_data,
        signature,
        credential,
        expected_user_handle,
        response_user_handle,
        user_handle_mode,
        user_verification,
    } = value;
    validate_user_handle(&expected_user_handle, true)?;
    if let Some(value) = &response_user_handle {
        validate_user_handle(value, true)?;
    }
    let verifier = verifier(relying_party.id, relying_party.origin)?;
    let challenge = Challenge::from_bytes(challenge).map_err(display)?;
    let credential = decode_credential(credential)?;
    let user_handle = match user_handle_mode {
        UserHandleMode::Identified => UserHandleVerification::for_identified_user(
            &expected_user_handle,
            response_user_handle.as_deref(),
        ),
        UserHandleMode::Discoverable => UserHandleVerification::for_discoverable_credential(
            &expected_user_handle,
            response_user_handle.as_deref(),
        ),
    };
    let request = AuthenticationVerification::new(
        &challenge,
        &credential_id,
        &client_data_json,
        &authenticator_data,
        &signature,
        &credential,
        user_handle,
    )
    .with_user_verification(policy(user_verification));
    let verified = verifier.verify_authentication(request).map_err(display)?;
    Ok(VerifiedAuthentication {
        credential: encode_credential(verified.credential())?,
        user_verified: verified.user_verified(),
        sign_count_advanced: verified.sign_count_advanced(),
    })
}

fn verifier(id: String, origin: String) -> Result<PasskeyRp, String> {
    RelyingParty::new(id, origin)
        .map(PasskeyRp::new)
        .map_err(display)
}

fn require_algorithm(value: &str) -> Result<(), String> {
    if value == ALGORITHM {
        Ok(())
    } else {
        Err(format!("unsupported WebAuthn algorithm {value:?}"))
    }
}

fn policy(value: UserVerification) -> UserVerificationPolicy {
    match value {
        UserVerification::Required => UserVerificationPolicy::Required,
        UserVerification::Preferred => UserVerificationPolicy::Preferred,
    }
}

fn encode_credential(value: &StoredCredential) -> Result<WireCredential, String> {
    Ok(WireCredential {
        id: value.credential_id().to_vec(),
        algorithm: ALGORITHM.into(),
        public_key_cose: value.public_key().to_cose_key().map_err(display)?,
        sign_count: value.sign_count(),
        backup_eligible: value.backup_eligible(),
        backup_state: value.backup_state(),
    })
}

fn decode_credential(value: WireCredential) -> Result<StoredCredential, String> {
    require_algorithm(&value.algorithm)?;
    let key = PublicKey::from_cose_key(&value.public_key_cose).map_err(display)?;
    let backup = CredentialBackupState::from_flags(value.backup_eligible, value.backup_state)
        .map_err(display)?;
    StoredCredential::new(value.id, key, value.sign_count, backup).map_err(display)
}

fn validate_user_handle(value: &[u8], required: bool) -> Result<(), String> {
    if value.len() <= MAX_USER_HANDLE_BYTES && (!required || !value.is_empty()) {
        Ok(())
    } else {
        Err("invalid WebAuthn user handle".into())
    }
}

fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}
