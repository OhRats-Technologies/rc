use super::persistence::{load_lock, snapshot_hash};
use super::{ControlAuthority, LockError, now_ms};
use rc_crypto::verify_ed25519;
use rc_protocol::{AuthoritySnapshot, McpGrantPayload};
use std::path::Path;

pub fn verify_mcp_grant(
    dir: &Path,
    grant_json: &str,
    signature: &str,
    control: &ControlAuthority,
    user_id: &str,
    device_id: &str,
) -> Result<McpGrantPayload, LockError> {
    let lock = load_lock(dir)?;
    let snapshot: AuthoritySnapshot =
        serde_json::from_str(&lock.snapshot).map_err(|_| LockError::Snapshot)?;
    let grant: McpGrantPayload =
        serde_json::from_str(grant_json).map_err(|_| LockError::McpGrant)?;
    let now = now_ms();
    if grant.v != 1
        || grant.id.is_empty()
        || grant.user_id != user_id
        || grant.user_id != control.grant.user_id
        || grant.issued_at > now + 60_000
        || (grant.expires_at != 0
            && (grant.expires_at <= now
                || grant.expires_at <= grant.issued_at
                || grant.expires_at - grant.issued_at > 366_i64 * 24 * 60 * 60 * 1000))
        || !grant.device_ids.iter().any(|id| id == device_id)
        || !grant.scopes.iter().any(|scope| scope == "mcp:terminal")
    {
        return Err(LockError::McpGrant);
    }
    let digest = snapshot_hash(grant_json);
    if !snapshot
        .mcp_grants
        .iter()
        .any(|entry| entry.id == grant.id && entry.user_id == user_id && entry.hash == digest)
    {
        return Err(LockError::McpGrant);
    }
    verify_ed25519(
        &control.grant.signing_public_key,
        format!("rc-mcp-grant-v1\n{digest}").as_bytes(),
        signature,
    )
    .map_err(|_| LockError::McpSignature)?;
    Ok(grant)
}
