wit_bindgen::generate!({
    path: "../../wit",
    world: "artifact-cache-fixture-local-storage",
    generate_all,
});

use exports::ohrats::rc_artifact_cache::local_storage::Guest as StorageGuest;
use ohrats::{rc_artifact_cache::types::Artifact, rc_plugin::types::Service};

const LOCAL_DIGEST: &str =
    "sha256:3071910d02cb4b93c5bf83d2f04eabbd1b1f25062ca6f161e8f60453c96b1f48";
const TAMPERED_DIGEST: &str =
    "sha256:5d4553c6b682104be56b7da1ddf97ae3913e29432fb169a13a2310cc861dc36f";

struct LocalStorageFixture;

impl Guest for LocalStorageFixture {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:artifact-cache-fixture-local-storage".into(),
            version: "0.1.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-artifact-cache/local-storage".into(),
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

impl StorageGuest for LocalStorageFixture {
    fn read(digest: String, max_bytes: u64) -> Result<Option<Artifact>, String> {
        let (stored_digest, bytes) = match digest.as_str() {
            LOCAL_DIGEST => (LOCAL_DIGEST, b"local-cache-hit".as_slice()),
            TAMPERED_DIGEST => (TAMPERED_DIGEST, b"tampered-response".as_slice()),
            _ => return Ok(None),
        };
        if bytes.len() as u64 > max_bytes {
            return Err("fixture local artifact exceeds requested capacity".into());
        }
        Ok(Some(Artifact {
            digest: stored_digest.into(),
            bytes: bytes.to_vec(),
        }))
    }
}

export!(LocalStorageFixture);
