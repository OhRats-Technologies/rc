wit_bindgen::generate!({
    path: "../../wit",
    world: "authority-fixture",
    generate_all,
});

use ed25519_dalek::{Signature, Signer, SigningKey, Verifier, VerifyingKey};
use exports::ohrats::rc_crypto::verifier::Guest as VerifierGuest;
use ohrats::{
    rc_crypto::types::{PublicKey, SignatureAlgorithm},
    rc_plugin::types::Service,
};
use sha2::{Digest, Sha256};

struct AuthorityCryptoFixture;

impl Guest for AuthorityCryptoFixture {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:authority-fixture".into(),
            version: "0.1.0".into(),
            provides: vec![service("ohrats:rc-crypto/verifier")],
            requires: Vec::new(),
            commands: vec![
                command(
                    "authority-fixture-public",
                    "Print the deterministic fixture control public key",
                    "rc authority-fixture-public",
                ),
                command(
                    "authority-fixture-sign",
                    "Sign one deterministic authority fixture payload",
                    "rc authority-fixture-sign <payload>",
                ),
            ],
        }
    }
    fn activate() -> Result<(), String> {
        Ok(())
    }
    fn deactivate() {}
    fn invoke(command: String, args: Vec<String>) -> Result<u32, String> {
        match (command.as_str(), args.as_slice()) {
            ("authority-fixture-public", []) => {
                println!("{}", hex(&fixture_key().verifying_key().to_bytes()));
                Ok(0)
            }
            ("authority-fixture-sign", [payload]) => {
                println!(
                    "{}",
                    hex(&fixture_key().sign(payload.as_bytes()).to_bytes())
                );
                Ok(0)
            }
            _ => Err(format!("unsupported command {command:?}")),
        }
    }
}

impl VerifierGuest for AuthorityCryptoFixture {
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

fn service(name: &str) -> Service {
    Service {
        name: name.into(),
        version: "0.1.0".into(),
        priority: 100,
        keys: Vec::new(),
    }
}

fn command(name: &str, summary: &str, usage: &str) -> ohrats::rc_plugin::types::Command {
    ohrats::rc_plugin::types::Command {
        name: name.into(),
        summary: summary.into(),
        usage: usage.into(),
    }
}

fn fixture_key() -> SigningKey {
    SigningKey::from_bytes(&[0x44; 32])
}

fn hex(value: &[u8]) -> String {
    value.iter().map(|byte| format!("{byte:02x}")).collect()
}

export!(AuthorityCryptoFixture);
