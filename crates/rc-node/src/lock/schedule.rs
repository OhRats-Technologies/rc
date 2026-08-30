use super::{LockError, now_ms};
use rc_protocol::{AuthorityScheduleGrant, AuthoritySnapshot};
use std::path::Path;

pub fn schedule_authority(
    dir: &Path,
    schedule_id: &str,
    device_id: &str,
    spec_hash: &str,
) -> Result<AuthorityScheduleGrant, LockError> {
    let state = super::load_lock(dir)?;
    let snapshot: AuthoritySnapshot =
        serde_json::from_str(&state.snapshot).map_err(|_| LockError::Snapshot)?;
    let grant = snapshot
        .schedule_grants
        .into_iter()
        .find(|grant| grant.schedule_id == schedule_id)
        .ok_or(LockError::ScheduleGrant)?;
    if grant.device_id != device_id
        || grant.spec_hash != spec_hash
        || (grant.expires_at != 0 && grant.expires_at <= now_ms())
        || !snapshot
            .members
            .iter()
            .any(|member| member.user_id == grant.user_id && member.role == "owner")
    {
        return Err(LockError::ScheduleGrant);
    }
    Ok(grant)
}
