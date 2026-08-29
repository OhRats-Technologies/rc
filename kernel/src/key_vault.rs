use crate::{
    bindings::ohrats::rc_keys::{
        host_custody::{Host, HostSecretKey, SecretKey},
        types::{KeyAlgorithm, PublicKey},
    },
    config,
    host::HostState,
};
use ed25519_dalek::{Signer, SigningKey};
use sha2::{Digest, Sha256};
use std::{
    collections::BTreeMap,
    fs,
    io::Write,
    path::{Path, PathBuf},
};
use wasmtime::component::Resource;

const KEY_BYTES: usize = 32;
const MAX_SLOT_BYTES: usize = 128;
const MAX_SIGN_BYTES: usize = 2 * 1024 * 1024;

pub struct ProtectedKey {
    key: SigningKey,
}

#[derive(Default)]
pub(crate) struct KeyHandles {
    next: u32,
    values: BTreeMap<u32, ProtectedKey>,
}

impl KeyHandles {
    fn insert(&mut self, value: ProtectedKey) -> Result<Resource<SecretKey>, String> {
        let rep = self
            .next
            .checked_add(1)
            .ok_or("protected key handle space exhausted")?;
        self.next = rep;
        self.values.insert(rep, value);
        Ok(Resource::new_own(rep))
    }

    fn get(&self, key: &Resource<SecretKey>) -> Result<&ProtectedKey, String> {
        self.values
            .get(&key.rep())
            .ok_or_else(|| "unknown protected key handle".to_owned())
    }

    fn remove(&mut self, key: Resource<SecretKey>) -> wasmtime::Result<()> {
        if self.values.remove(&key.rep()).is_none() {
            return Err(wasmtime::Error::msg("unknown protected key handle"));
        }
        Ok(())
    }
}

impl Host for HostState {
    fn open(
        &mut self,
        slot: String,
        algorithm: KeyAlgorithm,
    ) -> Result<Option<Resource<SecretKey>>, String> {
        validate_slot(&slot)?;
        ensure_ed25519(algorithm)?;
        let path = key_path(self, &slot)?;
        let Some(key) = load(&path)? else {
            return Ok(None);
        };
        self.key_handles.insert(ProtectedKey { key }).map(Some)
    }

    fn ensure(
        &mut self,
        slot: String,
        algorithm: KeyAlgorithm,
    ) -> Result<Resource<SecretKey>, String> {
        validate_slot(&slot)?;
        ensure_ed25519(algorithm)?;
        let path = key_path(self, &slot)?;
        if let Some(key) = load(&path)? {
            return self.key_handles.insert(ProtectedKey { key });
        }
        let mut bytes = [0_u8; KEY_BYTES];
        getrandom::fill(&mut bytes).map_err(display)?;
        let generated = SigningKey::from_bytes(&bytes);
        let created = persist_new(&path, &bytes)?;
        bytes.fill(0);
        let key = if created {
            generated
        } else {
            load(&path)?.ok_or_else(|| "protected key creation raced with removal".to_owned())?
        };
        self.key_handles.insert(ProtectedKey { key })
    }

    fn remove(&mut self, slot: String, algorithm: KeyAlgorithm) -> Result<bool, String> {
        validate_slot(&slot)?;
        ensure_ed25519(algorithm)?;
        let path = key_path(self, &slot)?;
        match fs::remove_file(&path) {
            Ok(()) => {
                sync_directory(path.parent().ok_or("protected key path has no parent")?)?;
                Ok(true)
            }
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(false),
            Err(error) => Err(error.to_string()),
        }
    }
}

impl HostSecretKey for HostState {
    fn public_key(&mut self, key: Resource<SecretKey>) -> Result<PublicKey, String> {
        let key = self.key_handles.get(&key)?;
        Ok(PublicKey {
            algorithm: KeyAlgorithm::Ed25519,
            bytes: key.key.verifying_key().to_bytes().to_vec(),
        })
    }

    fn sign(&mut self, key: Resource<SecretKey>, payload: Vec<u8>) -> Result<Vec<u8>, String> {
        if payload.len() > MAX_SIGN_BYTES {
            return Err("protected-key signing payload exceeds 2 MiB".into());
        }
        let key = self.key_handles.get(&key)?;
        Ok(key.key.sign(&payload).to_bytes().to_vec())
    }

    fn drop(&mut self, key: Resource<SecretKey>) -> wasmtime::Result<()> {
        self.key_handles.remove(key)
    }
}

fn key_path(state: &HostState, slot: &str) -> Result<PathBuf, String> {
    let owner = digest(state.plugin_id().as_bytes());
    let slot = digest(slot.as_bytes());
    let directory = state.environment.state_dir.join("keys").join(owner);
    config::prepare_private_dir(&directory).map_err(display)?;
    Ok(directory.join(format!("{slot}.ed25519")))
}

fn load(path: &Path) -> Result<Option<SigningKey>, String> {
    match fs::symlink_metadata(path) {
        Ok(metadata) => {
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
        }
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(None),
        Err(error) => return Err(error.to_string()),
    }
    match fs::read(path) {
        Ok(bytes) => {
            let secret: [u8; KEY_BYTES] = bytes
                .as_slice()
                .try_into()
                .map_err(|_| "protected key file has invalid length".to_owned())?;
            Ok(Some(SigningKey::from_bytes(&secret)))
        }
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(None),
        Err(error) => Err(error.to_string()),
    }
}

fn persist_new(path: &Path, bytes: &[u8; KEY_BYTES]) -> Result<bool, String> {
    let parent = path
        .parent()
        .ok_or_else(|| "protected key path has no parent".to_owned())?;
    config::prepare_private_dir(parent).map_err(display)?;
    let temporary = parent.join(format!(
        ".key.{}.{}.tmp",
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map_err(display)?
            .as_nanos()
    ));
    write_private(&temporary, bytes)?;
    match fs::hard_link(&temporary, path) {
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

fn write_private(path: &Path, bytes: &[u8]) -> Result<(), String> {
    let mut options = fs::OpenOptions::new();
    options.write(true).create_new(true);
    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt as _;
        options.mode(0o600);
    }
    let mut file = options.open(path).map_err(|error| error.to_string())?;
    file.write_all(bytes).map_err(|error| error.to_string())?;
    file.sync_all().map_err(|error| error.to_string())?;
    Ok(())
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

fn ensure_ed25519(algorithm: KeyAlgorithm) -> Result<(), String> {
    match algorithm {
        KeyAlgorithm::Ed25519 => Ok(()),
    }
}

fn validate_slot(slot: &str) -> Result<(), String> {
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

fn digest(value: &[u8]) -> String {
    format!("{:x}", Sha256::digest(value))
}

fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}

#[cfg(test)]
mod tests {
    use super::validate_slot;

    #[test]
    fn slot_names_are_bounded_and_non_path_authoritative() {
        assert!(validate_slot("node:device/identity").is_ok());
        assert!(validate_slot("").is_err());
        assert!(validate_slot("bad slot").is_err());
    }
}
