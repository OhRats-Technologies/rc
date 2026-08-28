wit_bindgen::generate!({
    path: "../../wit",
    world: "local-source",
});

use exports::ohrats::rc_plugin::package_source::{Guest as SourceGuest, PackageArtifact};
use ohrats::rc_plugin::types::{Command, Service};
use sha2::{Digest, Sha256};

struct LocalSource;

impl Guest for LocalSource {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:local-source".into(),
            version: "0.1.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-plugin/package-source".into(),
                version: "0.1.0".into(),
                priority: 100,
                keys: vec!["file".into()],
            }],
            requires: Vec::new(),
            commands: vec![Command {
                name: "source-file".into(),
                summary: "Describe the local-file package source".into(),
                usage: "rc source-file".into(),
            }],
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }

    fn deactivate() {}

    fn invoke(command: String, _args: Vec<String>) -> Result<u32, String> {
        if command != "source-file" {
            return Err(format!("unsupported command {command:?}"));
        }
        println!("file:<path> or a direct ./relative/path.wasm");
        Ok(0)
    }
}

impl SourceGuest for LocalSource {
    fn resolve(scheme: String, spec: String) -> Result<PackageArtifact, String> {
        if scheme != "file" {
            return Err(format!("local source does not handle {scheme:?}"));
        }
        let bytes = ohrats::rc_plugin::local_files::read(&spec)?;
        let digest = format!("sha256:{:x}", Sha256::digest(&bytes));
        let name = component_name(&spec)?;
        Ok(PackageArtifact {
            name,
            source: format!("file:{spec}"),
            digest,
            bytes,
        })
    }
}

fn component_name(path: &str) -> Result<String, String> {
    let filename = path
        .rsplit(['/', '\\'])
        .next()
        .filter(|value| !value.is_empty())
        .ok_or_else(|| "local source has no filename".to_owned())?;
    let name = filename.strip_suffix(".wasm").unwrap_or(filename);
    if name.is_empty()
        || name.len() > 96
        || !name
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'_' | b'.'))
    {
        return Err(format!("invalid component filename {filename:?}"));
    }
    Ok(name.into())
}

export!(LocalSource);
