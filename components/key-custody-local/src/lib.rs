wit_bindgen::generate!({
    path: "../../wit",
    world: "key-custody-local",
    generate_all,
});

use exports::ohrats::rc_keys::custody::Guest as CustodyGuest;
use ohrats::{
    rc_keys::{
        host_custody,
        types::{KeyAlgorithm, PublicKey},
    },
    rc_plugin::types::Service,
};

struct KeyCustodyLocal;

impl Guest for KeyCustodyLocal {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:key-custody-local".into(),
            version: "0.1.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-keys/custody".into(),
                version: "0.1.0".into(),
                priority: 100,
                keys: Vec::new(),
            }],
            requires: Vec::new(),
            commands: Vec::new(),
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }
    fn deactivate() {}
    fn invoke(command: String, _args: Vec<String>) -> Result<u32, String> {
        Err(format!("unsupported command {command:?}"))
    }
}

impl CustodyGuest for KeyCustodyLocal {
    fn ensure(slot: String, algorithm: KeyAlgorithm) -> Result<PublicKey, String> {
        host_custody::ensure(&slot, algorithm)?.public_key()
    }

    fn lookup(slot: String, algorithm: KeyAlgorithm) -> Result<Option<PublicKey>, String> {
        host_custody::open(&slot, algorithm)?
            .map(|key| key.public_key())
            .transpose()
    }

    fn sign(slot: String, algorithm: KeyAlgorithm, payload: Vec<u8>) -> Result<Vec<u8>, String> {
        let key = host_custody::open(&slot, algorithm)?
            .ok_or_else(|| "protected key slot does not exist".to_owned())?;
        key.sign(&payload)
    }

    fn remove(slot: String, algorithm: KeyAlgorithm) -> Result<bool, String> {
        host_custody::remove(&slot, algorithm)
    }
}

export!(KeyCustodyLocal);
