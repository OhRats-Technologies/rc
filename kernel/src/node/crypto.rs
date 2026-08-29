use super::values;
use crate::{
    descriptor::SelectionMode,
    runtime::Runtime,
    service::{PinnedProvider, ServiceRegistry},
};
use semver::VersionReq;
use std::{thread, time::Duration};
use wasmtime::component::{ResourceAny, Val};

const SERVICE: &str = "ohrats:rc-crypto/control";
const VERSION: &str = "^0.1";
const CLIENT_PUBLIC: &str = "B6N8vBQgk8i3VdwbEOhstCY3StFqqFPtC9_AsrhtHHw";
const PLAINTEXT: &[u8] = b"control-crypto-probe";

pub struct ComponentControlCrypto {
    registry: ServiceRegistry,
    requirement: VersionReq,
}

pub struct OpenedSession {
    pub session: ControlSession,
    pub static_public_key: String,
    pub ephemeral_public_key: String,
}

pub struct ControlSession {
    provider: PinnedProvider,
    resource: ResourceAny,
}

impl ComponentControlCrypto {
    pub fn new(registry: ServiceRegistry) -> anyhow::Result<Self> {
        Ok(Self {
            registry,
            requirement: VersionReq::parse(VERSION)?,
        })
    }

    pub fn available(&self) -> Result<bool, String> {
        self.registry
            .has_provider(SERVICE, &self.requirement, None)
            .map_err(display)
    }

    pub fn static_public(&self, device_id: &str) -> Result<String, String> {
        let values = self
            .registry
            .call_one(
                SERVICE,
                &self.requirement,
                SelectionMode::Single,
                "static-public",
                &[Val::String(device_id.to_owned())],
            )
            .map_err(display)?;
        string_result(values, "control static public key")
    }

    pub fn open_node(
        &self,
        device_id: &str,
        client_id: &str,
        challenge: &str,
        client_public_key: &str,
    ) -> Result<OpenedSession, String> {
        let provider = self
            .registry
            .pinned(SERVICE, &self.requirement)
            .map_err(display)?
            .into_iter()
            .next()
            .ok_or_else(|| "control crypto provider is unavailable".to_owned())?;
        let values = provider
            .call(
                SERVICE,
                "open-node",
                &[
                    Val::String(device_id.to_owned()),
                    Val::String(client_id.to_owned()),
                    Val::String(challenge.to_owned()),
                    Val::String(client_public_key.to_owned()),
                ],
            )
            .map_err(display)?;
        let mut fields = values::record(
            values::result_value(values, "control open-node")?,
            "control node session",
        )?;
        let resource = match take_field(&mut fields, "session")? {
            Val::Resource(resource) if resource.owned() => resource,
            _ => return Err("control provider returned an invalid session resource".into()),
        };
        let static_public_key = take_string(&mut fields, "static-public-key")?;
        let ephemeral_public_key = take_string(&mut fields, "ephemeral-public-key")?;
        Ok(OpenedSession {
            session: ControlSession { provider, resource },
            static_public_key,
            ephemeral_public_key,
        })
    }
}

impl ControlSession {
    pub fn encrypt(
        &self,
        direction: u8,
        sequence: u64,
        session_id: &str,
        label: &str,
        plaintext: &[u8],
    ) -> Result<String, String> {
        let values = self.call(
            "encrypt",
            direction,
            sequence,
            session_id,
            label,
            Val::List(plaintext.iter().copied().map(Val::U8).collect()),
        )?;
        string_result(values, "control encrypt")
    }

    pub fn decrypt(
        &self,
        direction: u8,
        sequence: u64,
        session_id: &str,
        label: &str,
        ciphertext: &str,
    ) -> Result<Vec<u8>, String> {
        let values = self.call(
            "decrypt",
            direction,
            sequence,
            session_id,
            label,
            Val::String(ciphertext.to_owned()),
        )?;
        let value = values::result_value(values, "control decrypt")?;
        values::list(value, "control plaintext")?
            .into_iter()
            .map(|value| match value {
                Val::U8(byte) => Ok(byte),
                _ => Err("control provider returned non-byte plaintext".into()),
            })
            .collect()
    }

    fn call(
        &self,
        function: &str,
        direction: u8,
        sequence: u64,
        session_id: &str,
        label: &str,
        payload: Val,
    ) -> Result<Vec<Val>, String> {
        self.provider
            .call(
                SERVICE,
                function,
                &[
                    Val::Resource(self.resource),
                    Val::U8(direction),
                    Val::U64(sequence),
                    Val::String(session_id.to_owned()),
                    Val::String(label.to_owned()),
                    payload,
                ],
            )
            .map_err(display)
    }
}

impl Drop for ControlSession {
    fn drop(&mut self) {
        if let Err(error) = self.provider.drop_resource(self.resource) {
            eprintln!("control session resource drop failed: {error:#}");
        }
    }
}

pub fn check(runtime: &Runtime) -> anyhow::Result<()> {
    let crypto = ComponentControlCrypto::new(runtime.service_registry())?;
    anyhow::ensure!(
        crypto.available().map_err(anyhow::Error::msg)?,
        "control crypto provider is unavailable"
    );
    let opened = crypto
        .open_node(
            "probe-device",
            "probe-client",
            "probe-challenge",
            CLIENT_PUBLIC,
        )
        .map_err(anyhow::Error::msg)?;
    anyhow::ensure!(
        opened.static_public_key.len() == 43 && opened.ephemeral_public_key.len() == 43,
        "control crypto returned an invalid X25519 public key"
    );
    anyhow::ensure!(
        opened.static_public_key
            == crypto
                .static_public("probe-device")
                .map_err(anyhow::Error::msg)?,
        "control crypto returned an unstable static public key"
    );
    roundtrip(&opened.session).map_err(anyhow::Error::msg)?;
    println!("control crypto: ok");
    Ok(())
}

pub fn probe(mut runtime: Runtime) -> anyhow::Result<()> {
    use std::io::{BufRead as _, Write as _};

    let crypto = ComponentControlCrypto::new(runtime.service_registry())?;
    thread::spawn(move || {
        loop {
            if let Err(error) = runtime.reconcile() {
                eprintln!("control crypto probe reconcile failed: {error:#}");
            }
            thread::sleep(Duration::from_millis(50));
        }
    });
    let mut session = None;
    for line in std::io::stdin().lock().lines() {
        match line?.as_str() {
            "available" => println!(
                "{}",
                if crypto.available().map_err(anyhow::Error::msg)? {
                    "Available"
                } else {
                    "Unavailable"
                }
            ),
            "open" => match crypto.open_node(
                "probe-device",
                "probe-client",
                "probe-challenge",
                CLIENT_PUBLIC,
            ) {
                Ok(opened) => {
                    println!("Open {}", opened.static_public_key);
                    session = Some(opened.session);
                }
                Err(_) => println!("Unavailable"),
            },
            "roundtrip" => match session.as_ref() {
                Some(session) => {
                    roundtrip(session).map_err(anyhow::Error::msg)?;
                    println!("Roundtrip ok");
                }
                None => println!("No session"),
            },
            "close" => {
                session = None;
                println!("Closed");
            }
            _ => println!("Unknown"),
        }
        std::io::stdout().flush()?;
    }
    Ok(())
}

fn roundtrip(session: &ControlSession) -> Result<(), String> {
    let ciphertext = session.encrypt(1, 7, "probe-session", "c2n", PLAINTEXT)?;
    let plaintext = session.decrypt(1, 7, "probe-session", "c2n", &ciphertext)?;
    if plaintext != PLAINTEXT {
        return Err("control crypto roundtrip mismatch".into());
    }
    Ok(())
}

fn string_result(values: Vec<Val>, label: &str) -> Result<String, String> {
    match values::result_value(values, label)? {
        Val::String(value) => Ok(value),
        _ => Err(format!("{label} returned a non-string value")),
    }
}

fn take_field(fields: &mut Vec<(String, Val)>, name: &str) -> Result<Val, String> {
    let index = fields
        .iter()
        .position(|(candidate, _)| candidate == name)
        .ok_or_else(|| format!("missing field {name:?}"))?;
    Ok(fields.remove(index).1)
}

fn take_string(fields: &mut Vec<(String, Val)>, name: &str) -> Result<String, String> {
    match take_field(fields, name)? {
        Val::String(value) => Ok(value),
        _ => Err(format!("field {name:?} is not a string")),
    }
}

fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}
