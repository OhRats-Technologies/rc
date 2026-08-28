wit_bindgen::generate!({
    path: "../../wit",
    world: "storage-fixture",
    generate_all,
});

use ohrats::{
    rc_plugin::types::Command,
    rc_storage::{
        durable_store,
        types::{Change, CommitError, Deletion, Entry},
    },
};

struct StorageFixture;

impl Guest for StorageFixture {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:storage-fixture".into(),
            version: "0.1.0".into(),
            provides: Vec::new(),
            requires: Vec::new(),
            commands: vec![
                command(
                    "kv-set",
                    "Set one component-private value",
                    "rc kv-set <bucket> <key> <value>",
                ),
                command(
                    "kv-get",
                    "Read one component-private value",
                    "rc kv-get <bucket> <key>",
                ),
                command(
                    "kv-list",
                    "List a component-private key prefix",
                    "rc kv-list <bucket> [prefix]",
                ),
                command(
                    "kv-delete",
                    "Delete one component-private value",
                    "rc kv-delete <bucket> <key>",
                ),
                command(
                    "kv-conflict",
                    "Prove optimistic transaction conflict detection",
                    "rc kv-conflict",
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
            "kv-set" => set(&args),
            "kv-get" => get(&args),
            "kv-list" => list(&args),
            "kv-delete" => delete(&args),
            "kv-conflict" => conflict(&args),
            _ => Err(format!("unsupported command {command:?}")),
        }
    }
}

fn command(name: &str, summary: &str, usage: &str) -> Command {
    Command {
        name: name.into(),
        summary: summary.into(),
        usage: usage.into(),
    }
}

fn set(args: &[String]) -> Result<u32, String> {
    let [bucket, key, value] = args else {
        return Err("usage: rc kv-set <bucket> <key> <value>".into());
    };
    let revision = durable_store::revision()?;
    durable_store::commit(
        revision,
        &[Change::Put(Entry {
            bucket: bucket.clone(),
            key: key.as_bytes().to_vec(),
            value: value.as_bytes().to_vec(),
        })],
    )
    .map_err(commit_error)?;
    Ok(0)
}

fn get(args: &[String]) -> Result<u32, String> {
    let [bucket, key] = args else {
        return Err("usage: rc kv-get <bucket> <key>".into());
    };
    let value = durable_store::get(bucket, key.as_bytes())?
        .ok_or_else(|| format!("key {key:?} is not set"))?;
    println!("{}", String::from_utf8_lossy(&value));
    Ok(0)
}

fn list(args: &[String]) -> Result<u32, String> {
    let (bucket, prefix) = match args {
        [bucket] => (bucket.as_str(), ""),
        [bucket, prefix] => (bucket.as_str(), prefix.as_str()),
        _ => return Err("usage: rc kv-list <bucket> [prefix]".into()),
    };
    let page = durable_store::scan(bucket, prefix.as_bytes(), None, 100)?;
    for entry in page.entries {
        println!(
            "{}={}",
            String::from_utf8_lossy(&entry.key),
            String::from_utf8_lossy(&entry.value)
        );
    }
    Ok(0)
}

fn delete(args: &[String]) -> Result<u32, String> {
    let [bucket, key] = args else {
        return Err("usage: rc kv-delete <bucket> <key>".into());
    };
    let revision = durable_store::revision()?;
    durable_store::commit(
        revision,
        &[Change::Delete(Deletion {
            bucket: bucket.clone(),
            key: key.as_bytes().to_vec(),
        })],
    )
    .map_err(commit_error)?;
    Ok(0)
}

fn conflict(args: &[String]) -> Result<u32, String> {
    if !args.is_empty() {
        return Err("usage: rc kv-conflict".into());
    }
    let revision = durable_store::revision()?;
    durable_store::commit(
        revision,
        &[Change::Put(Entry {
            bucket: "conflict".into(),
            key: b"first".to_vec(),
            value: b"one".to_vec(),
        })],
    )
    .map_err(commit_error)?;
    match durable_store::commit(
        revision,
        &[Change::Put(Entry {
            bucket: "conflict".into(),
            key: b"second".to_vec(),
            value: b"two".to_vec(),
        })],
    ) {
        Err(CommitError::Conflict(current)) => {
            println!("conflict at revision {current}");
            Ok(0)
        }
        Err(error) => Err(commit_error(error)),
        Ok(_) => Err("stale durable-store commit unexpectedly succeeded".into()),
    }
}

fn commit_error(error: CommitError) -> String {
    match error {
        CommitError::Conflict(current) => format!("data changed at revision {current}"),
        CommitError::Failure(error) => error,
    }
}

export!(StorageFixture);
