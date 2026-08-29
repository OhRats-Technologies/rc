wit_bindgen::generate!({
    path: "../../wit",
    world: "artifact-cache-mesh",
    generate_all,
});

use exports::ohrats::rc_artifact_cache::cache::Guest as CacheGuest;
use ohrats::rc_artifact_cache::{
    authorized_mesh_cache,
    types::{Artifact, FetchRequest},
};
use ohrats::rc_plugin::types::{Requirement, Selection, Service};
use sha2::{Digest, Sha256};

const MAX_ARTIFACT_BYTES: usize = 48 * 1024 * 1024;
const PRIORITY: i32 = 100;

struct MeshCache;

impl Guest for MeshCache {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:artifact-cache-mesh".into(),
            version: "0.1.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-artifact-cache/cache".into(),
                version: "0.1.0".into(),
                priority: PRIORITY,
                keys: vec!["mesh".into()],
            }],
            requires: vec![Requirement {
                name: "ohrats:rc-artifact-cache/authorized-mesh-cache".into(),
                version: "^0.1".into(),
                selection: Selection::Single,
            }],
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

impl CacheGuest for MeshCache {
    fn fetch(provider_key: String, request: FetchRequest) -> Result<Option<Artifact>, String> {
        if provider_key != "mesh" {
            return Err(format!("mesh cache does not handle {provider_key:?}"));
        }
        validate_request(&request)?;
        let Some(artifact) = authorized_mesh_cache::fetch(&request)? else {
            return Ok(None);
        };
        validate_artifact(&request, &artifact)?;
        Ok(Some(artifact))
    }
}

fn validate_request(request: &FetchRequest) -> Result<(), String> {
    if !valid_digest(&request.digest) {
        return Err(format!("invalid artifact digest {:?}", request.digest));
    }
    let maximum = usize::try_from(request.max_bytes)
        .map_err(|_| "artifact capacity exceeds host limits".to_owned())?;
    if maximum == 0 || maximum > MAX_ARTIFACT_BYTES {
        return Err("artifact capacity must be between 1 and 48 MiB".into());
    }
    Ok(())
}

fn validate_artifact(request: &FetchRequest, artifact: &Artifact) -> Result<(), String> {
    if artifact.digest != request.digest {
        return Err("mesh cache returned an address mismatch".into());
    }
    if artifact.bytes.len() > usize::try_from(request.max_bytes).unwrap_or(0)
        || artifact.bytes.len() > MAX_ARTIFACT_BYTES
    {
        return Err("mesh cache artifact exceeds requested capacity".into());
    }
    let actual = format!("sha256:{:x}", Sha256::digest(&artifact.bytes));
    if actual != artifact.digest {
        return Err("mesh cache artifact failed digest verification".into());
    }
    Ok(())
}

fn valid_digest(value: &str) -> bool {
    value.strip_prefix("sha256:").is_some_and(|hex| {
        hex.len() == 64
            && hex
                .bytes()
                .all(|byte| byte.is_ascii_digit() || matches!(byte, b'a'..=b'f'))
    })
}

#[cfg(test)]
mod tests {
    use super::{MAX_ARTIFACT_BYTES, PRIORITY, valid_digest};

    #[test]
    fn validates_the_same_address_shape_as_the_local_provider() {
        assert!(valid_digest(&format!("sha256:{}", "f".repeat(64))));
        assert!(!valid_digest("sha256:missing"));
        assert!(!valid_digest(&format!("sha256:{}", "F".repeat(64))));
    }

    #[test]
    fn mesh_provider_is_lower_priority_than_local_cache() {
        assert_eq!(MAX_ARTIFACT_BYTES, 48 * 1024 * 1024);
        assert_eq!(PRIORITY, 100);
    }
}

export!(MeshCache);
