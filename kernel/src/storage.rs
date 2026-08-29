mod component;
mod fs;
mod transaction;

use crate::{
    bindings::ohrats::rc_plugin::{
        artifact_cache::Host as ArtifactCacheHost,
        catalog_store::Host as CatalogStoreHost,
        component_store::{
            Host as ComponentStoreHost, InstalledComponent, PreparedSet, UpdateCandidate,
        },
        local_files::Host as LocalFilesHost,
        state_store::Host as StateStoreHost,
    },
    host::HostState,
};
use std::fs as std_fs;

impl ComponentStoreHost for HostState {
    fn installed(&mut self) -> Result<Vec<InstalledComponent>, String> {
        component::installed(&self.environment).map_err(display)
    }

    fn install(
        &mut self,
        name: String,
        artifact: Vec<u8>,
        expected_digest: Option<String>,
    ) -> Result<InstalledComponent, String> {
        component::install(
            &self.environment,
            &name,
            &artifact,
            expected_digest.as_deref(),
        )
        .map_err(display)
    }

    fn prepare(&mut self, candidates: Vec<UpdateCandidate>) -> Result<PreparedSet, String> {
        transaction::prepare(
            &self.environment,
            &candidates
                .into_iter()
                .map(transaction::Candidate::from)
                .collect::<Vec<_>>(),
        )
        .map(PreparedSet::from)
        .map_err(display)
    }

    fn commit(&mut self, prepared: PreparedSet) -> Result<Vec<InstalledComponent>, String> {
        transaction::commit(&self.environment, &prepared).map_err(display)
    }

    fn abort(&mut self, prepared: PreparedSet) {
        transaction::abort(&self.environment, &prepared);
    }

    fn remove(&mut self, name: String) -> Result<bool, String> {
        component::remove(&self.environment, &name).map_err(display)
    }
}

impl ArtifactCacheHost for HostState {
    fn read(&mut self, digest: String) -> Result<Option<Vec<u8>>, String> {
        transaction::recover(&self.environment).map_err(display)?;
        let path = fs::cache_path(&self.environment, &digest).map_err(display)?;
        match std_fs::read(path) {
            Ok(value) => {
                fs::verify_digest(&digest, &value).map_err(display)?;
                Ok(Some(value))
            }
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(None),
            Err(error) => Err(error.to_string()),
        }
    }

    fn write(&mut self, digest: String, artifact: Vec<u8>) -> Result<(), String> {
        if artifact.len() > fs::MAX_ARTIFACT_BYTES {
            return Err("artifact exceeds 48 MiB".into());
        }
        fs::verify_digest(&digest, &artifact).map_err(display)?;
        let path = fs::cache_path(&self.environment, &digest).map_err(display)?;
        fs::atomic_write(&path, &artifact).map_err(display)
    }
}

impl StateStoreHost for HostState {
    fn read(&mut self, name: String) -> Result<Option<Vec<u8>>, String> {
        let path = private_state_path(self, &name).map_err(display)?;
        match std_fs::read(path) {
            Ok(value) if value.len() <= fs::MAX_STATE_BYTES => Ok(Some(value)),
            Ok(_) => Err("component state exceeds 2 MiB".into()),
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(None),
            Err(error) => Err(error.to_string()),
        }
    }

    fn write(&mut self, name: String, value: Vec<u8>) -> Result<(), String> {
        if value.len() > fs::MAX_STATE_BYTES {
            return Err("component state exceeds 2 MiB".into());
        }
        let path = private_state_path(self, &name).map_err(display)?;
        fs::atomic_write(&path, &value).map_err(display)
    }

    fn remove(&mut self, name: String) -> Result<bool, String> {
        let path = private_state_path(self, &name).map_err(display)?;
        match std_fs::remove_file(path) {
            Ok(()) => Ok(true),
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(false),
            Err(error) => Err(error.to_string()),
        }
    }
}

impl LocalFilesHost for HostState {
    fn read(&mut self, path: String) -> Result<Vec<u8>, String> {
        let metadata = std_fs::metadata(&path).map_err(|error| error.to_string())?;
        if !metadata.is_file() {
            return Err(format!("{path:?} is not a file"));
        }
        if metadata.len() > fs::MAX_ARTIFACT_BYTES as u64 {
            return Err("local artifact exceeds 48 MiB".into());
        }
        std_fs::read(path).map_err(|error| error.to_string())
    }
}

impl CatalogStoreHost for HostState {
    fn read(&mut self, namespace: String) -> Result<Option<Vec<u8>>, String> {
        fs::validate_token(&namespace, "catalog namespace").map_err(display)?;
        let path = self
            .environment
            .catalog_dir
            .join(format!("{namespace}.toml"));
        match std_fs::read(path) {
            Ok(value) if value.len() <= fs::MAX_STATE_BYTES => Ok(Some(value)),
            Ok(_) => Err("catalog exceeds 2 MiB".into()),
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(None),
            Err(error) => Err(error.to_string()),
        }
    }
}

fn private_state_path(state: &HostState, name: &str) -> anyhow::Result<std::path::PathBuf> {
    fs::validate_token(name, "state name")?;
    let owner = state
        .plugin_id()
        .chars()
        .map(|character| {
            if character.is_ascii_alphanumeric() {
                character
            } else {
                '_'
            }
        })
        .collect::<String>();
    let directory = state.environment.state_dir.join("components").join(owner);
    std::fs::create_dir_all(&directory)?;
    Ok(directory.join(name))
}

fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}
