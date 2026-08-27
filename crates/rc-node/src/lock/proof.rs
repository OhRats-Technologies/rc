use super::persistence::load_lock;
use super::{ApiControlAuthority, ControlAuthority, LockError, now_ms};
use rc_crypto::{WebauthnProofError, control_grant_challenge, verify_webauthn_assertion};
use rc_protocol::{
    AuthorityApiKey, AuthorityMember, AuthoritySnapshot, ControlGrant, ControlProof,
};
use std::path::Path;

pub fn api_control_authority(dir: &Path, key_id: &str) -> Result<ApiControlAuthority, LockError> {
    let lock = load_lock(dir)?;
    let snapshot: AuthoritySnapshot =
        serde_json::from_str(&lock.snapshot).map_err(|_| LockError::Snapshot)?;
    let key = snapshot
        .api_keys
        .iter()
        .find(|key| key.id == key_id && (key.expires_at == 0 || key.expires_at > now_ms()))
        .ok_or(LockError::ApiKey)?;
    let member = member_for_user(&snapshot.members, &key.user_id).ok_or(LockError::ApiKey)?;
    if member.role == "viewer" || !matches!(member.role.as_str(), "owner" | "operator") {
        return Err(LockError::ApiKey);
    }
    let can_execute = has_scope(key, "execute");
    let can_manage_devices = member.role == "owner" && has_scope(key, "manage-devices");
    if !can_execute && !can_manage_devices {
        return Err(LockError::ApiKey);
    }
    Ok(ApiControlAuthority {
        user_id: key.user_id.clone(),
        role: member.role.clone(),
        public_key: key.public_key.clone(),
        can_execute,
        can_manage_devices,
    })
}

pub fn verify_control_proof(
    snapshot: &AuthoritySnapshot,
    proof: &ControlProof,
    origin: &str,
    rp_id: &str,
) -> Result<ControlAuthority, LockError> {
    let grant: ControlGrant = serde_json::from_str(&proof.grant).map_err(|_| LockError::Grant)?;
    if grant.v != 1 || grant.client_id.is_empty() || grant.user_id.is_empty() {
        return Err(LockError::Grant);
    }
    let now = now_ms();
    if grant.issued_at > now + 60_000
        || (grant.expires_at != 0
            && (grant.expires_at <= now
                || grant.expires_at <= grant.issued_at
                || grant.expires_at - grant.issued_at > 366_i64 * 24 * 60 * 60 * 1000))
    {
        return Err(LockError::GrantExpired);
    }
    let (member, credential) = snapshot
        .members
        .iter()
        .find_map(|member| {
            member
                .credentials
                .iter()
                .find(|credential| credential.id == proof.credential_id)
                .map(|credential| (member, credential))
        })
        .ok_or(LockError::Credential)?;
    if member.user_id != grant.user_id
        || member.role == "viewer"
        || !matches!(member.role.as_str(), "owner" | "operator")
    {
        return Err(LockError::Credential);
    }
    verify_webauthn_assertion(
        &proof.assertion,
        &proof.credential_id,
        &credential.public_key,
        &control_grant_challenge(&proof.grant),
        origin,
        rp_id,
    )
    .map_err(|error| match error {
        WebauthnProofError::Assertion => LockError::Assertion,
        WebauthnProofError::CredentialMismatch => LockError::CredentialMismatch,
        WebauthnProofError::StoredPasskey => LockError::StoredPasskey,
        WebauthnProofError::Verification => LockError::Passkey,
    })?;
    Ok(ControlAuthority {
        grant,
        role: member.role.clone(),
    })
}

pub fn hosted_control_authority(
    dir: &Path,
    proof: &ControlProof,
    user_id: &str,
) -> Result<ControlAuthority, LockError> {
    let lock = load_lock(dir)?;
    let snapshot: AuthoritySnapshot =
        serde_json::from_str(&lock.snapshot).map_err(|_| LockError::Snapshot)?;
    let authority = verify_control_proof(&snapshot, proof, &lock.origin, &lock.rp_id)?;
    if authority.grant.user_id != user_id || authority.role == "viewer" {
        return Err(LockError::Credential);
    }
    Ok(authority)
}

fn member_for_user<'a>(
    members: &'a [AuthorityMember],
    user_id: &str,
) -> Option<&'a AuthorityMember> {
    members.iter().find(|member| member.user_id == user_id)
}

fn has_scope(key: &AuthorityApiKey, scope: &str) -> bool {
    key.scopes.iter().any(|value| value == scope)
}
