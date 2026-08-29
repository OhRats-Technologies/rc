wit_bindgen::generate!({
    path: "../../wit",
    world: "artifact-cache-fixture-provider-v2",
    generate_all,
});

use exports::ohrats::rc_artifact_cache::cache::Guest as CacheGuest;
use ohrats::{
    rc_artifact_cache::types::{Artifact, FetchRequest},
    rc_plugin::types::Service,
};

const REPLACEMENT_DIGEST: &str =
    "sha256:af0c7881a2c728065860f8e59b3f0da442dcc78476a464a40fa328d23eab3f8f";

struct ReplacementProvider;

impl Guest for ReplacementProvider {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:artifact-cache-fixture-provider-v2".into(),
            version: "0.2.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-artifact-cache/cache".into(),
                version: "0.1.0".into(),
                priority: 300,
                keys: vec!["local".into()],
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

impl CacheGuest for ReplacementProvider {
    fn fetch(provider_key: String, request: FetchRequest) -> Result<Option<Artifact>, String> {
        if provider_key != "local" || request.digest != REPLACEMENT_DIGEST {
            return Ok(None);
        }
        let bytes = b"replacement-cache-hit";
        if bytes.len() as u64 > request.max_bytes {
            return Err("replacement artifact exceeds requested capacity".into());
        }
        Ok(Some(Artifact {
            digest: REPLACEMENT_DIGEST.into(),
            bytes: bytes.to_vec(),
        }))
    }
}

export!(ReplacementProvider);
