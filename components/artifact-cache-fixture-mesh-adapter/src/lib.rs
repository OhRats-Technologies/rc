wit_bindgen::generate!({
    path: "../../wit",
    world: "artifact-cache-fixture-mesh-adapter",
    generate_all,
});

use exports::ohrats::rc_artifact_cache::authorized_mesh_cache::Guest as MeshGuest;
use ohrats::{
    rc_artifact_cache::types::{Artifact, FetchRequest},
    rc_plugin::types::Service,
};

const MESH_DIGEST: &str = "sha256:917b79b2b68d8dce68c0a52ffffd3fab1b2b3e354fb13136b708d2ce55ed5f97";
const UNAUTHORIZED_DIGEST: &str =
    "sha256:a266213b292d9bc7c62a79025d8d4e2931e5c17781037bb14a3156ca2c45361e";

struct MeshAdapterFixture;

impl Guest for MeshAdapterFixture {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:artifact-cache-fixture-mesh-adapter".into(),
            version: "0.1.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-artifact-cache/authorized-mesh-cache".into(),
                version: "0.1.0".into(),
                priority: 100,
                keys: Vec::new(),
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

impl MeshGuest for MeshAdapterFixture {
    fn fetch(request: FetchRequest) -> Result<Option<Artifact>, String> {
        if request.digest == UNAUTHORIZED_DIGEST {
            return Err("mesh cache request is not authorized".into());
        }
        if request.digest != MESH_DIGEST {
            return Ok(None);
        }
        let bytes = b"mesh-cache-hit";
        if bytes.len() as u64 > request.max_bytes {
            return Err("fixture mesh artifact exceeds requested capacity".into());
        }
        Ok(Some(Artifact {
            digest: MESH_DIGEST.into(),
            bytes: bytes.to_vec(),
        }))
    }
}

export!(MeshAdapterFixture);
