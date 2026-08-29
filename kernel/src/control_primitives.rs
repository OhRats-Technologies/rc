use crate::{
    bindings::ohrats::rc_keys::host_custody::{HostSessionKey, SessionKey, SharedSecret},
    host::HostState,
};
use aes_gcm::{
    Aes256Gcm, KeyInit,
    aead::{Aead, Payload},
};
use hkdf::Hkdf;
use sha2::Sha256;
use std::collections::BTreeMap;
use wasmtime::component::Resource;

const KEY_BYTES: usize = 32;
const NONCE_BYTES: usize = 12;
const MAX_PAYLOAD_BYTES: usize = 2 * 1024 * 1024;
const MAX_AAD_BYTES: usize = 8 * 1024;
const MAX_SALT_BYTES: usize = 64;
const MAX_INFO_BYTES: usize = 1024;

#[derive(Default)]
pub(crate) struct SessionKeyHandles {
    next: u32,
    values: BTreeMap<u32, [u8; KEY_BYTES]>,
}

impl SessionKeyHandles {
    fn insert(&mut self, key: [u8; KEY_BYTES]) -> Result<Resource<SessionKey>, String> {
        let rep = self
            .next
            .checked_add(1)
            .ok_or("session-key handle space exhausted")?;
        self.next = rep;
        self.values.insert(rep, key);
        Ok(Resource::new_own(rep))
    }

    fn get(&self, key: &Resource<SessionKey>) -> Result<&[u8; KEY_BYTES], String> {
        self.values
            .get(&key.rep())
            .ok_or_else(|| "unknown session-key handle".to_owned())
    }

    fn remove(&mut self, key: Resource<SessionKey>) -> wasmtime::Result<()> {
        if self.values.remove(&key.rep()).is_none() {
            return Err(wasmtime::Error::msg("unknown session-key handle"));
        }
        Ok(())
    }
}

pub(crate) fn derive(
    state: &mut HostState,
    first: Resource<SharedSecret>,
    second: Resource<SharedSecret>,
    salt: Vec<u8>,
    info: Vec<u8>,
) -> Result<Resource<SessionKey>, String> {
    if salt.len() > MAX_SALT_BYTES || info.len() > MAX_INFO_BYTES {
        return Err("control KDF parameters are too large".into());
    }
    let first = state.key_handles.shared_bytes(&first)?;
    let second = state.key_handles.shared_bytes(&second)?;
    let mut material = [0_u8; KEY_BYTES * 2];
    material[..KEY_BYTES].copy_from_slice(&first);
    material[KEY_BYTES..].copy_from_slice(&second);
    let hkdf = Hkdf::<Sha256>::new(Some(&salt), &material);
    let mut key = [0_u8; KEY_BYTES];
    hkdf.expand(&info, &mut key)
        .map_err(|_| "control key derivation failed".to_owned())?;
    material.fill(0);
    state.control_keys.insert(key)
}

impl HostSessionKey for HostState {
    fn encrypt(
        &mut self,
        key: Resource<SessionKey>,
        nonce: Vec<u8>,
        aad: Vec<u8>,
        plaintext: Vec<u8>,
    ) -> Result<Vec<u8>, String> {
        crypt(self.control_keys.get(&key)?, nonce, aad, plaintext, true)
    }

    fn decrypt(
        &mut self,
        key: Resource<SessionKey>,
        nonce: Vec<u8>,
        aad: Vec<u8>,
        ciphertext: Vec<u8>,
    ) -> Result<Vec<u8>, String> {
        crypt(self.control_keys.get(&key)?, nonce, aad, ciphertext, false)
    }

    fn drop(&mut self, key: Resource<SessionKey>) -> wasmtime::Result<()> {
        self.control_keys.remove(key)
    }
}

fn crypt(
    key: &[u8; KEY_BYTES],
    nonce: Vec<u8>,
    aad: Vec<u8>,
    payload: Vec<u8>,
    encrypt: bool,
) -> Result<Vec<u8>, String> {
    if payload.len() > MAX_PAYLOAD_BYTES || aad.len() > MAX_AAD_BYTES {
        return Err("control cipher input is too large".into());
    }
    let nonce: [u8; NONCE_BYTES] = nonce
        .try_into()
        .map_err(|_| "control cipher nonce must be 12 bytes".to_owned())?;
    let cipher = Aes256Gcm::new_from_slice(key).map_err(|_| "invalid session key".to_owned())?;
    let payload = Payload {
        msg: &payload,
        aad: &aad,
    };
    if encrypt {
        cipher.encrypt((&nonce).into(), payload)
    } else {
        cipher.decrypt((&nonce).into(), payload)
    }
    .map_err(|_| "control frame authentication failed".to_owned())
}
