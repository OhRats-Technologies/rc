wit_bindgen::generate!({
    path: "../../wit",
    world: "authority-fixture",
    generate_all,
});

use ed25519_dalek::{Signer, SigningKey};

struct AuthorityCryptoFixture;

impl Guest for AuthorityCryptoFixture {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:authority-fixture".into(),
            version: "0.1.0".into(),
            provides: Vec::new(),
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
