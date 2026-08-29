use crate::{bindings::ohrats::rc_keys::types::KeyAlgorithm, config, host::HostState};
use sha2::{Digest, Sha256};
use std::{
    fs,
    io::Write,
    path::{Path, PathBuf},
};

const KEY_BYTES: usize = 32;
const MAX_SLOT_BYTES: usize = 128;

pub(super) fn load(
    state: &HostState,
    slot: &str,
    algorithm: KeyAlgorithm,
) -> Result<Option<[u8; KEY_BYTES]>, String> {
    let path = key_path(state, slot, algorithm)?;
    match fs::symlink_metadata(&path) {
        Ok(metadata) => validate_metadata(&metadata)?,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(None),
        Err(error) => return Err(error.to_string()),
    }
    match fs::read(path) {
        Ok(bytes) => bytes
            .try_into()
            .map(Some)
            .map_err(|_| "protected key file has invalid length".to_owned()),
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(None),
        Err(error) => Err(error.to_string()),
    }
}

pub(super) fn persist_new(
    state: &HostState,
    slot: &str,
    algorithm: KeyAlgorithm,
    bytes: &[u8; KEY_BYTES],
) -> Result<bool, String> {
    let path = key_path(state, slot, algorithm)?;
    let parent = path
        .parent()
        .ok_or_else(|| "protected key path has no parent".to_owned())?;
    let temporary = parent.join(format!(
        ".key.{}.{}.tmp",
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map_err(display)?
            .as_nanos()
    ));
    write_private(&temporary, bytes)?;
    match fs::hard_link(&temporary, &path) {
        Ok(()) => {
            let _ = fs::remove_file(&temporary);
            sync_directory(parent)?;
            Ok(true)
        }
        Err(error) if error.kind() == std::io::ErrorKind::AlreadyExists => {
            let _ = fs::remove_file(&temporary);
            Ok(false)
        }
        Err(error) => {
            let _ = fs::remove_file(&temporary);
            Err(error.to_string())
        }
    }
}

pub(super) fn remove(
    state: &HostState,
    slot: &str,
    algorithm: KeyAlgorithm,
) -> Result<bool, String> {
    let path = key_path(state, slot, algorithm)?;
    match fs::remove_file(&path) {
        Ok(()) => {
            sync_directory(path.parent().ok_or("protected key path has no parent")?)?;
            Ok(true)
        }
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(false),
        Err(error) => Err(error.to_string()),
    }
}

pub(super) fn validate_slot(slot: &str) -> Result<(), String> {
    if slot.is_empty()
        || slot.len() > MAX_SLOT_BYTES
        || !slot.bytes().all(|byte| {
            byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'_' | b'.' | b':' | b'/')
        })
    {
        Err("invalid protected key slot".into())
    } else {
        Ok(())
    }
}

fn key_path(state: &HostState, slot: &str, algorithm: KeyAlgorithm) -> Result<PathBuf, String> {
    let owner = digest(state.plugin_id().as_bytes());
    let slot = digest(slot.as_bytes());
    let directory = state.environment.state_dir.join("keys").join(owner);
    config::prepare_private_dir(&directory).map_err(display)?;
    Ok(directory.join(format!("{slot}.{}", suffix(algorithm))))
}

fn suffix(algorithm: KeyAlgorithm) -> &'static str {
    match algorithm {
        KeyAlgorithm::Ed25519 => "ed25519",
        KeyAlgorithm::X25519 => "x25519",
    }
}

fn validate_metadata(metadata: &fs::Metadata) -> Result<(), String> {
    if metadata.file_type().is_symlink() || !metadata.is_file() {
        return Err("protected key path is not a regular file".into());
    }
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt as _;
        if metadata.permissions().mode() & 0o077 != 0 {
            return Err("protected key file permissions are too broad".into());
        }
    }
    Ok(())
}

fn write_private(path: &Path, bytes: &[u8]) -> Result<(), String> {
    let mut options = fs::OpenOptions::new();
    options.write(true).create_new(true);
    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt as _;
        options.mode(0o600);
    }
    let mut file = options.open(path).map_err(display)?;
    file.write_all(bytes).map_err(display)?;
    file.sync_all().map_err(display)
}

#[cfg(unix)]
fn sync_directory(path: &Path) -> Result<(), String> {
    fs::File::open(path)
        .and_then(|file| file.sync_all())
        .map_err(display)
}

#[cfg(not(unix))]
fn sync_directory(_: &Path) -> Result<(), String> {
    Ok(())
}

fn digest(value: &[u8]) -> String {
    format!("{:x}", Sha256::digest(value))
}

fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}
