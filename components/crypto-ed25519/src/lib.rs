wit_bindgen::generate!({
    path: "../../wit",
    world: "crypto-ed25519",
    generate_all,
});

use ed25519_dalek::{Signature, Verifier, VerifyingKey};
use exports::ohrats::rc_crypto::verifier::Guest as VerifierGuest;
use ohrats::{
    rc_crypto::types::{PublicKey, SignatureAlgorithm},
    rc_plugin::types::Service,
};
use sha2::{Digest, Sha256};

struct CryptoEd25519;

impl Guest for CryptoEd25519 {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:crypto-ed25519".into(),
            version: "0.1.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-crypto/verifier".into(),
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

impl VerifierGuest for CryptoEd25519 {
    fn verify(
        algorithm: SignatureAlgorithm,
        public_key: PublicKey,
        payload: Vec<u8>,
        signature: Vec<u8>,
    ) -> Result<bool, String> {
        if algorithm != SignatureAlgorithm::Ed25519
            || public_key.algorithm != SignatureAlgorithm::Ed25519
        {
            return Ok(false);
        }
        let key: [u8; 32] = match public_key.bytes.as_slice().try_into() {
            Ok(value) => value,
            Err(_) => return Ok(false),
        };
        let signature: [u8; 64] = match signature.as_slice().try_into() {
            Ok(value) => value,
            Err(_) => return Ok(false),
        };
        Ok(VerifyingKey::from_bytes(&key)
            .map(|key| {
                key.verify(&payload, &Signature::from_bytes(&signature))
                    .is_ok()
            })
            .unwrap_or(false))
    }

    fn sha256(value: Vec<u8>) -> Vec<u8> {
        Sha256::digest(value).to_vec()
    }
}

#[cfg(test)]
mod tests {
    use ed25519_dalek::{Signer, SigningKey};
    use sha2::{Digest, Sha256};

    #[test]
    fn deterministic_vectors_match() {
        let key = SigningKey::from_bytes(&[0x44; 32]);
        let payload = b"rc-crypto-ed25519";
        let signature = key.sign(payload);
        key.verifying_key()
            .verify_strict(payload, &signature)
            .unwrap();
        assert_eq!(Sha256::digest(payload).len(), 32);
    }
}

export!(CryptoEd25519);
