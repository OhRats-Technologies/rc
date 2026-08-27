use super::LockError;
use super::persistence::{load_lock, save_lock, snapshot_hash, validate_snapshot};
use super::proof::verify_control_proof;
use rc_crypto::verify_ed25519;
use rc_protocol::{ControlProof, LockState};
use std::path::Path;

pub fn sync_lock(
    dir: &Path,
    snapshot_json: &str,
    previous_hash: &str,
    previous_generation: u64,
    proof: &ControlProof,
    signature: &str,
) -> Result<(), LockError> {
    let current = load_lock(dir)?;
    let old_snapshot = validate_snapshot(&current.snapshot)?;
    let next_snapshot = validate_snapshot(snapshot_json)?;
    if old_snapshot.workspace_id != next_snapshot.workspace_id {
        return Err(LockError::WorkspaceMismatch);
    }
    let current_hash = snapshot_hash(&current.snapshot);
    if previous_hash != current_hash || previous_generation != current.generation {
        return Err(LockError::StaleTransition);
    }
    let authority = verify_control_proof(&old_snapshot, proof, &current.origin, &current.rp_id)
        .map_err(|_| LockError::OwnerRequired)?;
    if authority.role != "owner" {
        return Err(LockError::OwnerRequired);
    }
    let payload = format!(
        "rc-authority-v3\n{}\n{}\n{}",
        current.generation,
        current_hash,
        snapshot_hash(snapshot_json)
    );
    verify_ed25519(
        &authority.grant.signing_public_key,
        payload.as_bytes(),
        signature,
    )
    .map_err(|_| LockError::AuthoritySignature)?;
    let generation = current
        .generation
        .checked_add(1)
        .ok_or(LockError::GenerationExhausted)?;
    save_lock(
        dir,
        &LockState {
            snapshot: snapshot_json.to_owned(),
            origin: current.origin,
            rp_id: current.rp_id,
            generation,
        },
    )
}
