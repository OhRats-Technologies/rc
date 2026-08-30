use super::LockError;
use rc_protocol::{AuthoritySnapshot, LockState};
use sha2::{Digest, Sha256};
use std::{fs, io, path::Path};
use url::Url;

pub fn load_lock(dir: &Path) -> Result<LockState, LockError> {
    let path = crate::lock_path(dir);
    rc_platform::validate_private_path(&path, false).map_err(|error| {
        if error.kind() == io::ErrorKind::NotFound {
            LockError::Missing
        } else {
            LockError::Io(error)
        }
    })?;
    let bytes = fs::read(path).map_err(|error| {
        if error.kind() == io::ErrorKind::NotFound {
            LockError::Missing
        } else {
            LockError::Io(error)
        }
    })?;
    let value: LockState = serde_json::from_slice(&bytes).map_err(|_| LockError::Corrupt)?;
    validate_snapshot(&value.snapshot)?;
    if value.origin.is_empty() || value.rp_id.is_empty() {
        return Err(LockError::Corrupt);
    }
    Ok(value)
}

pub fn bootstrap_lock(dir: &Path, snapshot: &str, server: &str) -> Result<(), LockError> {
    let path = crate::lock_path(dir);
    match fs::metadata(&path) {
        Ok(_) => return load_lock(dir).map(|_| ()).map_err(|_| LockError::Corrupt),
        Err(error) if error.kind() == io::ErrorKind::NotFound => {}
        Err(error) => return Err(error.into()),
    }
    validate_snapshot(snapshot)?;
    let parsed = Url::parse(server).map_err(|_| LockError::Origin)?;
    let host = parsed.host_str().ok_or(LockError::Origin)?;
    if !matches!(parsed.scheme(), "http" | "https") {
        return Err(LockError::Origin);
    }
    save_lock(
        dir,
        &LockState {
            snapshot: snapshot.to_owned(),
            origin: parsed.origin().ascii_serialization(),
            rp_id: host.to_owned(),
            generation: 0,
        },
    )
}

pub fn lock_metadata(dir: &Path) -> (String, u64) {
    let Ok(value) = load_lock(dir) else {
        return (String::new(), 0);
    };
    (snapshot_hash(&value.snapshot), value.generation)
}

pub fn snapshot_hash(snapshot: &str) -> String {
    Sha256::digest(snapshot.as_bytes())
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect()
}

pub(super) fn validate_snapshot(snapshot: &str) -> Result<AuthoritySnapshot, LockError> {
    let parsed: AuthoritySnapshot =
        serde_json::from_str(snapshot).map_err(|_| LockError::Snapshot)?;
    if parsed.v != 1 || parsed.workspace_id.is_empty() {
        return Err(LockError::Snapshot);
    }
    Ok(parsed)
}

pub(super) fn save_lock(dir: &Path, value: &LockState) -> Result<(), LockError> {
    fs::create_dir_all(dir)?;
    set_mode(dir, 0o700)?;
    let path = crate::lock_path(dir);
    let temporary = path.with_extension(format!("tmp-{}", std::process::id()));
    let mut bytes = serde_json::to_vec_pretty(value).map_err(io::Error::other)?;
    bytes.push(b'\n');
    fs::write(&temporary, bytes)?;
    set_mode(&temporary, 0o600)?;
    fs::rename(&temporary, &path)?;
    set_mode(&path, 0o600)?;
    Ok(())
}

fn set_mode(path: &Path, mode: u32) -> io::Result<()> {
    rc_platform::protect_private_path(path, mode & 0o100 != 0)
}
