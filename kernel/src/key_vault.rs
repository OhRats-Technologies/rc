use crate::{
    bindings::ohrats::rc_keys::{
        host_custody::{
            Host, HostSecretKey, HostSharedSecret, SecretKey, SessionKey, SharedSecret,
        },
        types::{KeyAlgorithm, PublicKey},
    },
    host::HostState,
};
use ed25519_dalek::{Signer, SigningKey};
use std::collections::BTreeMap;
use wasmtime::component::Resource;
use x25519_dalek::{PublicKey as X25519PublicKey, StaticSecret};

mod storage;

const KEY_BYTES: usize = 32;
const MAX_SIGN_BYTES: usize = 2 * 1024 * 1024;

enum ProtectedKey {
    Ed25519(SigningKey),
    X25519(StaticSecret),
}

struct SharedSecretValue {
    bytes: [u8; KEY_BYTES],
}

#[derive(Default)]
pub(crate) struct KeyHandles {
    next: u32,
    keys: BTreeMap<u32, ProtectedKey>,
    shared: BTreeMap<u32, SharedSecretValue>,
}

impl KeyHandles {
    fn next(&mut self) -> Result<u32, String> {
        let rep = self
            .next
            .checked_add(1)
            .ok_or("protected key handle space exhausted")?;
        self.next = rep;
        Ok(rep)
    }

    fn insert_key(&mut self, value: ProtectedKey) -> Result<Resource<SecretKey>, String> {
        let rep = self.next()?;
        self.keys.insert(rep, value);
        Ok(Resource::new_own(rep))
    }

    fn key(&self, key: &Resource<SecretKey>) -> Result<&ProtectedKey, String> {
        self.keys
            .get(&key.rep())
            .ok_or_else(|| "unknown protected key handle".to_owned())
    }

    fn remove_key(&mut self, key: Resource<SecretKey>) -> wasmtime::Result<()> {
        if self.keys.remove(&key.rep()).is_none() {
            return Err(wasmtime::Error::msg("unknown protected key handle"));
        }
        Ok(())
    }

    fn insert_shared(&mut self, bytes: [u8; KEY_BYTES]) -> Result<Resource<SharedSecret>, String> {
        let rep = self.next()?;
        self.shared.insert(rep, SharedSecretValue { bytes });
        Ok(Resource::new_own(rep))
    }

    pub(crate) fn shared_bytes(
        &self,
        value: &Resource<SharedSecret>,
    ) -> Result<[u8; KEY_BYTES], String> {
        self.shared
            .get(&value.rep())
            .map(|value| value.bytes)
            .ok_or_else(|| "unknown shared-secret handle".to_owned())
    }

    fn remove_shared(&mut self, value: Resource<SharedSecret>) -> wasmtime::Result<()> {
        if self.shared.remove(&value.rep()).is_none() {
            return Err(wasmtime::Error::msg("unknown shared-secret handle"));
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
        storage::validate_slot(&slot)?;
        let Some(key) = load_key(self, &slot, algorithm)? else {
            return Ok(None);
        };
        self.key_handles.insert_key(key).map(Some)
    }

    fn ensure(
        &mut self,
        slot: String,
        algorithm: KeyAlgorithm,
    ) -> Result<Resource<SecretKey>, String> {
        storage::validate_slot(&slot)?;
        if let Some(key) = load_key(self, &slot, algorithm)? {
            return self.key_handles.insert_key(key);
        }
        let mut bytes = random_bytes()?;
        let generated = key_from_bytes(algorithm, &bytes);
        let created = storage::persist_new(self, &slot, algorithm, &bytes)?;
        bytes.fill(0);
        let key = if created {
            generated
        } else {
            load_key(self, &slot, algorithm)?
                .ok_or_else(|| "protected key creation raced with removal".to_owned())?
        };
        self.key_handles.insert_key(key)
    }

    fn generate(&mut self, algorithm: KeyAlgorithm) -> Result<Resource<SecretKey>, String> {
        let mut bytes = random_bytes()?;
        let key = key_from_bytes(algorithm, &bytes);
        bytes.fill(0);
        self.key_handles.insert_key(key)
    }

    fn remove(&mut self, slot: String, algorithm: KeyAlgorithm) -> Result<bool, String> {
        storage::validate_slot(&slot)?;
        storage::remove(self, &slot, algorithm)
    }

    fn derive(
        &mut self,
        first: Resource<SharedSecret>,
        second: Resource<SharedSecret>,
        salt: Vec<u8>,
        info: Vec<u8>,
    ) -> Result<Resource<SessionKey>, String> {
        crate::control_primitives::derive(self, first, second, salt, info)
    }
}

impl HostSecretKey for HostState {
    fn public_key(&mut self, key: Resource<SecretKey>) -> Result<PublicKey, String> {
        public_key(self.key_handles.key(&key)?)
    }

    fn sign(&mut self, key: Resource<SecretKey>, payload: Vec<u8>) -> Result<Vec<u8>, String> {
        if payload.len() > MAX_SIGN_BYTES {
            return Err("protected-key signing payload exceeds 2 MiB".into());
        }
        match self.key_handles.key(&key)? {
            ProtectedKey::Ed25519(key) => Ok(key.sign(&payload).to_bytes().to_vec()),
            ProtectedKey::X25519(_) => Err("X25519 keys do not support signing".into()),
        }
    }

    fn agree(
        &mut self,
        key: Resource<SecretKey>,
        peer_public: Vec<u8>,
    ) -> Result<Resource<SharedSecret>, String> {
        let peer: [u8; KEY_BYTES] = peer_public
            .try_into()
            .map_err(|_| "invalid X25519 peer public key length".to_owned())?;
        let bytes = match self.key_handles.key(&key)? {
            ProtectedKey::X25519(secret) => *secret
                .diffie_hellman(&X25519PublicKey::from(peer))
                .as_bytes(),
            ProtectedKey::Ed25519(_) => return Err("Ed25519 keys do not support agreement".into()),
        };
        self.key_handles.insert_shared(bytes)
    }

    fn drop(&mut self, key: Resource<SecretKey>) -> wasmtime::Result<()> {
        self.key_handles.remove_key(key)
    }
}

impl HostSharedSecret for HostState {
    fn drop(&mut self, value: Resource<SharedSecret>) -> wasmtime::Result<()> {
        self.key_handles.remove_shared(value)
    }
}

fn load_key(
    state: &HostState,
    slot: &str,
    algorithm: KeyAlgorithm,
) -> Result<Option<ProtectedKey>, String> {
    let Some(mut bytes) = storage::load(state, slot, algorithm)? else {
        return Ok(None);
    };
    let key = key_from_bytes(algorithm, &bytes);
    bytes.fill(0);
    Ok(Some(key))
}

fn key_from_bytes(algorithm: KeyAlgorithm, bytes: &[u8; KEY_BYTES]) -> ProtectedKey {
    match algorithm {
        KeyAlgorithm::Ed25519 => ProtectedKey::Ed25519(SigningKey::from_bytes(bytes)),
        KeyAlgorithm::X25519 => ProtectedKey::X25519(StaticSecret::from(*bytes)),
    }
}

fn public_key(key: &ProtectedKey) -> Result<PublicKey, String> {
    match key {
        ProtectedKey::Ed25519(key) => Ok(PublicKey {
            algorithm: KeyAlgorithm::Ed25519,
            bytes: key.verifying_key().to_bytes().to_vec(),
        }),
        ProtectedKey::X25519(key) => Ok(PublicKey {
            algorithm: KeyAlgorithm::X25519,
            bytes: X25519PublicKey::from(key).as_bytes().to_vec(),
        }),
    }
}

fn random_bytes() -> Result<[u8; KEY_BYTES], String> {
    let mut bytes = [0_u8; KEY_BYTES];
    getrandom::fill(&mut bytes).map_err(|error| error.to_string())?;
    Ok(bytes)
}

#[cfg(test)]
mod tests {
    use super::storage::validate_slot;

    #[test]
    fn slot_names_are_bounded_and_non_path_authoritative() {
        assert!(validate_slot("node:device/identity").is_ok());
        assert!(validate_slot("").is_err());
        assert!(validate_slot("bad slot").is_err());
    }
}
