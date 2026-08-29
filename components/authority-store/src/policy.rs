use crate::model::{self, StoredSnapshot, StoredState};
use crate::ohrats::rc_authority::types::{
    Lock, OwnerAuthorization, Rejection, Snapshot, Transition,
};
use crate::ohrats::rc_crypto::{
    types::{PublicKey, SignatureAlgorithm},
    verifier,
};
use crate::{storage, validate};

pub fn initialize(value: Snapshot) -> Result<Lock, Rejection> {
    let snapshot = validate::snapshot(&value).map_err(Rejection::InvalidSnapshot)?;
    let state = StoredState {
        hash: model::snapshot_hash(&snapshot),
        snapshot,
        generation: 0,
        pending_invalidation: None,
    };
    if !storage::create(&state).map_err(Rejection::StorageFailure)? {
        return Err(Rejection::AlreadyInitialized);
    }
    Ok(state.lock())
}

pub fn current() -> Result<Option<Lock>, Rejection> {
    storage::load()
        .map(|state| state.map(|value| value.lock()))
        .map_err(Rejection::StorageFailure)
}

pub fn snapshot_hash(value: Snapshot) -> Result<String, Rejection> {
    let snapshot = validate::snapshot(&value).map_err(Rejection::InvalidSnapshot)?;
    Ok(model::snapshot_hash(&snapshot))
}

pub fn apply(value: Transition) -> Result<Lock, Rejection> {
    let next = validate::snapshot(&value.snapshot).map_err(Rejection::InvalidSnapshot)?;
    let current = storage::load()
        .map_err(Rejection::StorageFailure)?
        .ok_or(Rejection::NotInitialized)?;
    if current.snapshot.workspace_id != next.workspace_id {
        return Err(Rejection::WorkspaceMismatch);
    }
    if current.hash != value.parent_hash || current.generation != value.parent_generation {
        return Err(Rejection::StaleParent);
    }
    authorize(
        &current.snapshot,
        &value.authorization,
        &value.snapshot,
        &current.hash,
        current.generation,
    )?;
    let generation = current
        .generation
        .checked_add(1)
        .ok_or(Rejection::GenerationExhausted)?;
    let next_state = StoredState {
        hash: model::snapshot_hash(&next),
        snapshot: next,
        generation,
        pending_invalidation: Some(generation),
    };
    storage::replace(&current.hash, current.generation, &next_state).map_err(|error| {
        if error == "stale authority parent" {
            Rejection::StaleParent
        } else {
            Rejection::StorageFailure(error)
        }
    })?;
    Ok(next_state.lock())
}

pub fn take_invalidation_signal() -> Result<Option<u64>, Rejection> {
    let Some(mut current) = storage::load().map_err(Rejection::StorageFailure)? else {
        return Err(Rejection::NotInitialized);
    };
    let signal = current.pending_invalidation;
    let Some(signal) = signal else {
        return Ok(None);
    };
    current.pending_invalidation = None;
    storage::clear_signal(&current).map_err(Rejection::StorageFailure)?;
    Ok(Some(signal))
}

fn authorize(
    current: &StoredSnapshot,
    authorization: &OwnerAuthorization,
    next: &Snapshot,
    parent_hash: &str,
    generation: u64,
) -> Result<(), Rejection> {
    let Some(member) = current
        .members
        .iter()
        .find(|member| member.user_id == authorization.user_id && member.role.is_owner())
    else {
        return Err(Rejection::OwnerRequired);
    };
    if !member
        .passkeys
        .iter()
        .any(|key| key.credential_id == authorization.passkey_credential_id)
    {
        return Err(Rejection::OwnerRequired);
    }
    let Some(key) = member.control_keys.iter().find(|key| {
        key.id == authorization.control_key_id
            && key.authorized_by_passkey == authorization.passkey_credential_id
    }) else {
        return Err(Rejection::ControlKeyRequired);
    };
    let next_hash = {
        let checked = validate::snapshot(next).map_err(Rejection::InvalidSnapshot)?;
        model::snapshot_hash(&checked)
    };
    let payload = format!("rc-authority-v3\n{generation}\n{parent_hash}\n{next_hash}");
    let valid = verifier::verify(
        SignatureAlgorithm::Ed25519,
        &PublicKey {
            algorithm: SignatureAlgorithm::Ed25519,
            bytes: key.public_key.clone(),
        },
        payload.as_bytes(),
        &authorization.signature,
    )
    .map_err(|_| Rejection::SignatureInvalid)?;
    valid.then_some(()).ok_or(Rejection::SignatureInvalid)
}
