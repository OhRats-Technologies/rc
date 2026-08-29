wit_bindgen::generate!({
    path: "../../wit",
    world: "key-custody-fixture",
    generate_all,
});

use ohrats::{
    rc_crypto::{
        types::{PublicKey as CryptoPublicKey, SignatureAlgorithm},
        verifier,
    },
    rc_keys::{
        custody,
        types::{KeyAlgorithm, PublicKey},
    },
    rc_plugin::types::{Command, Requirement, Selection},
};

const PAYLOAD: &[u8] = b"rc-key-custody-fixture-v1";

struct KeyCustodyFixture;

impl Guest for KeyCustodyFixture {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:key-custody-fixture".into(),
            version: "0.1.0".into(),
            provides: Vec::new(),
            requires: vec![
                requirement("ohrats:rc-keys/custody"),
                requirement("ohrats:rc-crypto/verifier"),
            ],
            commands: vec![
                command(
                    "key-custody-public",
                    "Ensure a protected key and print only its public key",
                    "rc key-custody-public <slot>",
                ),
                command(
                    "key-custody-verify",
                    "Sign and verify a deterministic protected-key payload",
                    "rc key-custody-verify <slot>",
                ),
                command(
                    "key-custody-lookup",
                    "Print a protected key public key or missing",
                    "rc key-custody-lookup <slot>",
                ),
                command(
                    "key-custody-remove",
                    "Remove a protected key slot",
                    "rc key-custody-remove <slot>",
                ),
            ],
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }
    fn deactivate() {}

    fn invoke(command: String, args: Vec<String>) -> Result<u32, String> {
        let [slot] = args.as_slice() else {
            return Err(format!("usage: rc {command} <slot>"));
        };
        match command.as_str() {
            "key-custody-public" => print_public(&custody::ensure(slot, KeyAlgorithm::Ed25519)?),
            "key-custody-verify" => verify(slot),
            "key-custody-lookup" => lookup(slot),
            "key-custody-remove" => remove(slot),
            _ => Err(format!("unsupported command {command:?}")),
        }
    }
}

fn print_public(key: &PublicKey) -> Result<u32, String> {
    if key.algorithm != KeyAlgorithm::Ed25519 || key.bytes.len() != 32 {
        return Err("custody returned an invalid Ed25519 public key".into());
    }
    println!("{}", hex(&key.bytes));
    Ok(0)
}

fn verify(slot: &str) -> Result<u32, String> {
    let key = custody::ensure(slot, KeyAlgorithm::Ed25519)?;
    let signature = custody::sign(slot, KeyAlgorithm::Ed25519, PAYLOAD)?;
    let valid = verifier::verify(
        SignatureAlgorithm::Ed25519,
        &CryptoPublicKey {
            algorithm: SignatureAlgorithm::Ed25519,
            bytes: key.bytes,
        },
        PAYLOAD,
        &signature,
    )?;
    if !valid {
        return Err("custody signature did not verify".into());
    }
    println!("key custody fixture: ok");
    Ok(0)
}

fn lookup(slot: &str) -> Result<u32, String> {
    match custody::lookup(slot, KeyAlgorithm::Ed25519)? {
        Some(key) => print_public(&key),
        None => {
            println!("missing");
            Ok(0)
        }
    }
}

fn remove(slot: &str) -> Result<u32, String> {
    println!(
        "{}",
        if custody::remove(slot, KeyAlgorithm::Ed25519)? {
            "removed"
        } else {
            "missing"
        }
    );
    Ok(0)
}

fn command(name: &str, summary: &str, usage: &str) -> Command {
    Command {
        name: name.into(),
        summary: summary.into(),
        usage: usage.into(),
    }
}

fn requirement(name: &str) -> Requirement {
    Requirement {
        name: name.into(),
        version: "^0.1".into(),
        selection: Selection::Single,
    }
}

fn hex(value: &[u8]) -> String {
    value.iter().map(|byte| format!("{byte:02x}")).collect()
}

export!(KeyCustodyFixture);
