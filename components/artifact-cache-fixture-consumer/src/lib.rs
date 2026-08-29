wit_bindgen::generate!({
    path: "../../wit",
    world: "artifact-cache-fixture-consumer",
    generate_all,
});

use ohrats::{
    rc_artifact_cache::{
        cache,
        types::{Artifact, FetchRequest},
    },
    rc_plugin::{
        service_registry,
        types::{Command, Requirement, Selection},
    },
};

const SERVICE: &str = "ohrats:rc-artifact-cache/cache";
const LOCAL_DIGEST: &str =
    "sha256:3071910d02cb4b93c5bf83d2f04eabbd1b1f25062ca6f161e8f60453c96b1f48";
const MESH_DIGEST: &str = "sha256:917b79b2b68d8dce68c0a52ffffd3fab1b2b3e354fb13136b708d2ce55ed5f97";
const TAMPERED_DIGEST: &str =
    "sha256:5d4553c6b682104be56b7da1ddf97ae3913e29432fb169a13a2310cc861dc36f";
const UNAUTHORIZED_DIGEST: &str =
    "sha256:a266213b292d9bc7c62a79025d8d4e2931e5c17781037bb14a3156ca2c45361e";
const REPLACEMENT_DIGEST: &str =
    "sha256:af0c7881a2c728065860f8e59b3f0da442dcc78476a464a40fa328d23eab3f8f";

struct Consumer;

impl Guest for Consumer {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:artifact-cache-fixture-consumer".into(),
            version: "0.1.0".into(),
            provides: Vec::new(),
            requires: vec![Requirement {
                name: SERVICE.into(),
                version: "^0.1".into(),
                selection: Selection::Keyed,
            }],
            commands: commands(),
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }
    fn deactivate() {}
    fn invoke(command: String, _args: Vec<String>) -> Result<u32, String> {
        match command.as_str() {
            "cache-local" => show("local", fetch("local", LOCAL_DIGEST)?),
            "cache-fallback" => fallback(MESH_DIGEST),
            "cache-miss" => fallback(&format!("sha256:{}", "0".repeat(64))),
            "cache-unauthorized" => fallback(UNAUTHORIZED_DIGEST),
            "cache-tampered" => show("local", fetch("local", TAMPERED_DIGEST)?),
            "cache-oversized" => oversized(),
            "cache-priority" => priority(),
            "cache-replacement" => replacement(),
            other => Err(format!("unsupported command {other:?}")),
        }
    }
}

fn commands() -> Vec<Command> {
    [
        "cache-local",
        "cache-fallback",
        "cache-miss",
        "cache-unauthorized",
        "cache-tampered",
        "cache-oversized",
        "cache-priority",
        "cache-replacement",
    ]
    .into_iter()
    .map(|name| Command {
        name: name.into(),
        summary: "Exercise the artifact cache runtime contract".into(),
        usage: format!("rc {name}"),
    })
    .collect()
}

fn fetch(provider: &str, digest: &str) -> Result<Option<Artifact>, String> {
    cache::fetch(
        provider,
        &FetchRequest {
            digest: digest.into(),
            max_bytes: 1024,
        },
    )
}

fn fallback(digest: &str) -> Result<u32, String> {
    if let Some(artifact) = fetch("local", digest)? {
        return show("local", Some(artifact));
    }
    if let Some(artifact) = fetch("mesh", digest)? {
        return show("mesh", Some(artifact));
    }
    println!("registry:miss");
    Ok(0)
}

fn show(source: &str, artifact: Option<Artifact>) -> Result<u32, String> {
    let artifact = artifact.ok_or_else(|| format!("{source} cache miss"))?;
    let value = String::from_utf8(artifact.bytes).map_err(|_| "artifact is not UTF-8")?;
    println!("{source}:{value}");
    Ok(0)
}

fn oversized() -> Result<u32, String> {
    cache::fetch(
        "local",
        &FetchRequest {
            digest: LOCAL_DIGEST.into(),
            max_bytes: 48 * 1024 * 1024 + 1,
        },
    )?;
    Err("oversized cache request was accepted".into())
}

fn priority() -> Result<u32, String> {
    for provider in service_registry::providers(SERVICE, "^0.1")? {
        println!(
            "{}\t{}\t{}",
            provider.component_id,
            provider.priority,
            provider.keys.join(",")
        );
    }
    Ok(0)
}

fn replacement() -> Result<u32, String> {
    match fetch("local", REPLACEMENT_DIGEST)? {
        Some(_) => println!("replacement:hit"),
        None => println!("replacement:miss"),
    }
    Ok(0)
}

export!(Consumer);
