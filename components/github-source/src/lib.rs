wit_bindgen::generate!({
    path: "../../wit",
    world: "github-source",
});

mod release;
mod spec;

use exports::ohrats::rc_plugin::package_source::{Guest as SourceGuest, PackageArtifact};
use ohrats::rc_plugin::types::Service;

struct GithubSource;

impl Guest for GithubSource {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:github-source".into(),
            version: "0.1.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-plugin/package-source".into(),
                version: "0.1.0".into(),
                priority: 70,
                keys: vec!["github".into()],
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

impl SourceGuest for GithubSource {
    fn resolve(scheme: String, value: String) -> Result<PackageArtifact, String> {
        if scheme != "github" {
            return Err(format!("GitHub source does not handle {scheme:?}"));
        }
        let parsed = spec::GithubSpec::parse(&value)?;
        if parsed.path.ends_with(".wasm") {
            release::raw_component(&parsed)
        } else {
            release::release_component(&parsed)
        }
    }
}

export!(GithubSource);
