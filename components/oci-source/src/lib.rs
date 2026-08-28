wit_bindgen::generate!({
    path: "../../wit",
    world: "oci-source",
});

mod client;
mod spec;

use exports::ohrats::rc_plugin::package_source::{Guest as SourceGuest, PackageArtifact};
use ohrats::rc_plugin::types::Service;

struct OciSource;

impl Guest for OciSource {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:oci-source".into(),
            version: "0.1.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-plugin/package-source".into(),
                version: "0.1.0".into(),
                priority: 90,
                keys: vec!["oci".into()],
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

impl SourceGuest for OciSource {
    fn resolve(scheme: String, value: String) -> Result<PackageArtifact, String> {
        if scheme != "oci" {
            return Err(format!("OCI source does not handle {scheme:?}"));
        }
        client::pull(&spec::OciSpec::parse(&value)?)
    }
}

export!(OciSource);
