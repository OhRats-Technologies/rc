wit_bindgen::generate!({
    path: "../../wit",
    world: "authority-store",
    generate_all,
});

mod fixture;
mod model;
mod policy;
mod storage;
mod validate;

use exports::ohrats::rc_authority::policy::Guest as PolicyGuest;
use ohrats::{
    rc_authority::types::{Lock, OwnerAuthorization, Rejection, Snapshot, Transition},
    rc_plugin::types::{Requirement, Selection, Service},
};

struct AuthorityStore;

impl Guest for AuthorityStore {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:authority-store".into(),
            version: "0.1.0".into(),
            provides: vec![service("ohrats:rc-authority/policy")],
            requires: vec![Requirement {
                name: "ohrats:rc-crypto/verifier".into(),
                version: "^0.1".into(),
                selection: Selection::Single,
            }],
            commands: vec![
                command(
                    "authority-seed",
                    "Seed a deterministic authority snapshot",
                    "rc authority-seed <id>",
                ),
                command(
                    "authority-verify",
                    "Verify authority snapshot restart state",
                    "rc authority-verify <id>",
                ),
                command(
                    "authority-transition-payload",
                    "Print the deterministic authority transition payload",
                    "rc authority-transition-payload <id>",
                ),
                command(
                    "authority-apply",
                    "Apply the signed deterministic authority transition",
                    "rc authority-apply <id> <signature-hex>",
                ),
                command(
                    "authority-verify-transition",
                    "Verify signed transition and invalidation state",
                    "rc authority-verify-transition <id>",
                ),
            ],
        }
    }
    fn activate() -> Result<(), String> {
        Ok(())
    }
    fn deactivate() {}
    fn invoke(command: String, args: Vec<String>) -> Result<u32, String> {
        match command.as_str() {
            "authority-seed" => seed(&args),
            "authority-verify" => verify(&args),
            "authority-transition-payload" => transition_payload(&args),
            "authority-apply" => apply(&args),
            "authority-verify-transition" => verify_transition(&args),
            _ => Err(format!("unsupported command {command:?}")),
        }
    }
}

impl PolicyGuest for AuthorityStore {
    fn initialize(value: Snapshot) -> Result<Lock, Rejection> {
        policy::initialize(value)
    }
    fn current() -> Result<Option<Lock>, Rejection> {
        policy::current()
    }
    fn snapshot_hash(value: Snapshot) -> Result<String, Rejection> {
        policy::snapshot_hash(value)
    }
    fn apply(value: Transition) -> Result<Lock, Rejection> {
        policy::apply(value)
    }
    fn take_invalidation_signal() -> Result<Option<u64>, Rejection> {
        policy::take_invalidation_signal()
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

fn seed(args: &[String]) -> Result<u32, String> {
    let [fixture, public_key] = args else {
        return Err("usage: rc authority-seed <id> <control-public-key-hex>".into());
    };
    fixture::validate_id(fixture)?;
    let lock = policy::initialize(fixture::snapshot(
        fixture,
        fixture::decode_hex(public_key, 32)?,
    ))
    .map_err(|error| format!("{error:?}"))?;
    println!("{} {}", lock.generation, lock.hash);
    Ok(0)
}

fn transition_payload(args: &[String]) -> Result<u32, String> {
    let [fixture] = args else {
        return Err("usage: rc authority-transition-payload <id>".into());
    };
    let (lock, next) = transition_state(fixture)?;
    println!("{}", payload(&lock, &next)?);
    Ok(0)
}

fn apply(args: &[String]) -> Result<u32, String> {
    let [fixture, signature] = args else {
        return Err("usage: rc authority-apply <id> <signature-hex>".into());
    };
    let (lock, next) = transition_state(fixture)?;
    let transition = Transition {
        snapshot: next,
        parent_hash: lock.hash.clone(),
        parent_generation: lock.generation,
        authorization: OwnerAuthorization {
            user_id: "fixture-owner".into(),
            passkey_credential_id: "fixture-passkey".into(),
            control_key_id: "fixture-control".into(),
            signature: fixture::decode_hex(signature, 64)?,
        },
    };
    let next = policy::apply(transition.clone()).map_err(|error| format!("{error:?}"))?;
    if !matches!(policy::apply(transition), Err(Rejection::StaleParent)) {
        return Err("authority transition replay was accepted".into());
    }
    println!("{} {}", next.generation, next.hash);
    Ok(0)
}

fn verify_transition(args: &[String]) -> Result<u32, String> {
    let [fixture] = args else {
        return Err("usage: rc authority-verify-transition <id>".into());
    };
    fixture::validate_id(fixture)?;
    let lock = policy::current()
        .map_err(|error| format!("{error:?}"))?
        .ok_or("authority snapshot did not survive restart")?;
    if lock.generation != 1 || lock.snapshot.active_execution_mcp_grants.len() != 1 {
        return Err("signed authority transition did not persist".into());
    }
    if policy::take_invalidation_signal().map_err(|error| format!("{error:?}"))? != Some(1)
        || policy::take_invalidation_signal()
            .map_err(|error| format!("{error:?}"))?
            .is_some()
    {
        return Err("authority invalidation signal was not one-shot".into());
    }
    println!("authority transition: ok");
    Ok(0)
}

fn transition_state(fixture: &str) -> Result<(Lock, Snapshot), String> {
    fixture::validate_id(fixture)?;
    let lock = policy::current()
        .map_err(|error| format!("{error:?}"))?
        .ok_or("authority snapshot is not initialized")?;
    if lock.generation != 0 || lock.snapshot.workspace_id != format!("fixture-workspace-{fixture}")
    {
        return Err("authority fixture is not at generation zero".into());
    }
    let next = fixture::transitioned(lock.snapshot.clone());
    Ok((lock, next))
}

fn payload(lock: &Lock, next: &Snapshot) -> Result<String, String> {
    let next_hash = policy::snapshot_hash(next.clone()).map_err(|error| format!("{error:?}"))?;
    Ok(format!(
        "rc-authority-v3\n{}\n{}\n{}",
        lock.generation, lock.hash, next_hash
    ))
}

fn verify(args: &[String]) -> Result<u32, String> {
    let [fixture] = args else {
        return Err("usage: rc authority-verify <id>".into());
    };
    fixture::validate_id(fixture)?;
    let lock = policy::current()
        .map_err(|error| format!("{error:?}"))?
        .ok_or("authority snapshot did not survive restart")?;
    if lock.generation != 0
        || lock.snapshot.workspace_id != format!("fixture-workspace-{fixture}")
        || lock.snapshot.devices.len() != 1
        || lock.snapshot.devices[0].identity_public_key != vec![0x11; 32]
        || lock.snapshot.devices[0].transport_public_key != vec![0x22; 32]
    {
        return Err("authority snapshot restart state is invalid".into());
    }
    if policy::take_invalidation_signal()
        .map_err(|error| format!("{error:?}"))?
        .is_some()
    {
        return Err("TOFU initialization emitted an invalidation signal".into());
    }
    println!("authority state: ok");
    Ok(0)
}

export!(AuthorityStore);
