use crate::{
    bindings::ohrats::rc_plugin::{
        artifact_cache::Host as ArtifactCacheHost,
        catalog_store::Host as CatalogStoreHost,
        component_store::{Host as ComponentStoreHost, InstalledComponent},
        local_files::Host as LocalFilesHost,
        state_store::Host as StateStoreHost,
    },
    component::LoadedComponent,
    host::{HostEnvironment, HostState},
};
use sha2::{Digest, Sha256};
use std::{
    fs,
    path::{Path, PathBuf},
};

const MAX_ARTIFACT_BYTES: usize = 48 * 1024 * 1024;
const MAX_STATE_BYTES: usize = 2 * 1024 * 1024;

impl ComponentStoreHost for HostState {
    fn installed(&mut self) -> Result<Vec<InstalledComponent>, String> {
        installed(&self.environment).map_err(display)
    }

    fn install(
        &mut self,
        name: String,
        artifact: Vec<u8>,
        expected_digest: Option<String>,
    ) -> Result<InstalledComponent, String> {
        install(
            &self.environment,
            &name,
            &artifact,
            expected_digest.as_deref(),
        )
        .map_err(display)
    }

    fn remove(&mut self, name: String) -> Result<bool, String> {
        remove(&self.environment, &name).map_err(display)
    }
}

impl ArtifactCacheHost for HostState {
    fn read(&mut self, digest: String) -> Result<Option<Vec<u8>>, String> {
        let path = cache_path(&self.environment, &digest).map_err(display)?;
        match fs::read(path) {
            Ok(value) => {
                verify_digest(&digest, &value).map_err(display)?;
                Ok(Some(value))
            }
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(None),
            Err(error) => Err(error.to_string()),
        }
    }

    fn write(&mut self, digest: String, artifact: Vec<u8>) -> Result<(), String> {
        if artifact.len() > MAX_ARTIFACT_BYTES {
            return Err("artifact exceeds 48 MiB".into());
        }
        verify_digest(&digest, &artifact).map_err(display)?;
        let path = cache_path(&self.environment, &digest).map_err(display)?;
        atomic_write(&path, &artifact).map_err(display)
    }
}

impl StateStoreHost for HostState {
    fn read(&mut self, name: String) -> Result<Option<Vec<u8>>, String> {
        let path = private_state_path(self, &name).map_err(display)?;
        match fs::read(path) {
            Ok(value) if value.len() <= MAX_STATE_BYTES => Ok(Some(value)),
            Ok(_) => Err("component state exceeds 2 MiB".into()),
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(None),
            Err(error) => Err(error.to_string()),
        }
    }

    fn write(&mut self, name: String, value: Vec<u8>) -> Result<(), String> {
        if value.len() > MAX_STATE_BYTES {
            return Err("component state exceeds 2 MiB".into());
        }
        let path = private_state_path(self, &name).map_err(display)?;
        atomic_write(&path, &value).map_err(display)
    }

    fn remove(&mut self, name: String) -> Result<bool, String> {
        let path = private_state_path(self, &name).map_err(display)?;
        match fs::remove_file(path) {
            Ok(()) => Ok(true),
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(false),
            Err(error) => Err(error.to_string()),
        }
    }
}

impl LocalFilesHost for HostState {
    fn read(&mut self, path: String) -> Result<Vec<u8>, String> {
        let metadata = fs::metadata(&path).map_err(|error| error.to_string())?;
        if !metadata.is_file() {
            return Err(format!("{path:?} is not a file"));
        }
        if metadata.len() > MAX_ARTIFACT_BYTES as u64 {
            return Err("local artifact exceeds 48 MiB".into());
        }
        fs::read(path).map_err(|error| error.to_string())
    }
}

impl CatalogStoreHost for HostState {
    fn read(&mut self, namespace: String) -> Result<Option<Vec<u8>>, String> {
        validate_token(&namespace, "catalog namespace").map_err(display)?;
        let path = self
            .environment
            .catalog_dir
            .join(format!("{namespace}.toml"));
        match fs::read(path) {
            Ok(value) if value.len() <= MAX_STATE_BYTES => Ok(Some(value)),
            Ok(_) => Err("catalog exceeds 2 MiB".into()),
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(None),
            Err(error) => Err(error.to_string()),
        }
    }
}

fn installed(environment: &HostEnvironment) -> anyhow::Result<Vec<InstalledComponent>> {
    let mut values = Vec::new();
    let mut paths = fs::read_dir(environment.component_dir.as_ref())?
        .filter_map(Result::ok)
        .map(|entry| entry.path())
        .filter(|path| {
            path.extension()
                .is_some_and(|extension| extension == "wasm")
        })
        .collect::<Vec<_>>();
    paths.sort();
    for path in paths {
        let name = path
            .file_stem()
            .and_then(|value| value.to_str())
            .ok_or_else(|| anyhow::anyhow!("invalid component filename {}", path.display()))?;
        let bytes = fs::read(&path)?;
        let (id, version, digest) = LoadedComponent::inspect(environment, path.clone(), &bytes)?;
        values.push(InstalledComponent {
            name: name.into(),
            id,
            version: version.to_string(),
            managed: marker_matches(environment, name, &digest),
            digest,
        });
    }
    Ok(values)
}

fn install(
    environment: &HostEnvironment,
    name: &str,
    artifact: &[u8],
    expected_digest: Option<&str>,
) -> anyhow::Result<InstalledComponent> {
    validate_token(name, "component name")?;
    anyhow::ensure!(
        artifact.len() <= MAX_ARTIFACT_BYTES,
        "artifact exceeds 48 MiB"
    );
    let digest = format!("sha256:{:x}", Sha256::digest(artifact));
    if let Some(expected) = expected_digest {
        anyhow::ensure!(
            expected == digest,
            "artifact digest does not match expected digest"
        );
    }
    let component_path = environment.component_dir.join(format!("{name}.wasm"));
    let marker_path = marker_path(environment, name);
    anyhow::ensure!(
        !component_path.exists() || marker_path.is_file(),
        "refusing to replace unmanaged component {name:?}"
    );
    let (id, version, inspected_digest) =
        LoadedComponent::inspect(environment, component_path.clone(), artifact)?;
    anyhow::ensure!(
        inspected_digest == digest,
        "artifact digest changed during inspection"
    );
    atomic_write(&component_path, artifact)?;
    atomic_write(&marker_path, format!("{digest}\n").as_bytes())?;
    let cache = cache_path(environment, &digest)?;
    if !cache.is_file() {
        atomic_write(&cache, artifact)?;
    }
    Ok(InstalledComponent {
        name: name.into(),
        id,
        version: version.to_string(),
        digest,
        managed: true,
    })
}

fn remove(environment: &HostEnvironment, name: &str) -> anyhow::Result<bool> {
    validate_token(name, "component name")?;
    let component = environment.component_dir.join(format!("{name}.wasm"));
    if !component.exists() {
        return Ok(false);
    }
    let marker = marker_path(environment, name);
    anyhow::ensure!(
        marker.is_file(),
        "refusing to remove unmanaged component {name:?}"
    );
    fs::remove_file(component)?;
    match fs::remove_file(marker) {
        Ok(()) => {}
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
        Err(error) => return Err(error.into()),
    }
    Ok(true)
}

fn private_state_path(state: &HostState, name: &str) -> anyhow::Result<PathBuf> {
    validate_token(name, "state name")?;
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
    fs::create_dir_all(&directory)?;
    Ok(directory.join(name))
}

fn marker_path(environment: &HostEnvironment, name: &str) -> PathBuf {
    environment.component_dir.join(format!("{name}.managed"))
}

fn cache_path(environment: &HostEnvironment, digest: &str) -> anyhow::Result<PathBuf> {
    let value = digest
        .strip_prefix("sha256:")
        .filter(|value| value.len() == 64 && value.bytes().all(|byte| byte.is_ascii_hexdigit()))
        .ok_or_else(|| anyhow::anyhow!("invalid SHA-256 digest {digest:?}"))?;
    Ok(environment
        .cache_dir
        .join("sha256")
        .join(format!("{}.wasm", value.to_ascii_lowercase())))
}

fn verify_digest(expected: &str, artifact: &[u8]) -> anyhow::Result<()> {
    let actual = format!("sha256:{:x}", Sha256::digest(artifact));
    anyhow::ensure!(
        actual.eq_ignore_ascii_case(expected),
        "artifact digest mismatch"
    );
    Ok(())
}

fn marker_matches(environment: &HostEnvironment, name: &str, digest: &str) -> bool {
    fs::read_to_string(marker_path(environment, name)).is_ok_and(|value| value.trim() == digest)
}

fn atomic_write(path: &Path, value: &[u8]) -> anyhow::Result<()> {
    let parent = path
        .parent()
        .ok_or_else(|| anyhow::anyhow!("path has no parent"))?;
    fs::create_dir_all(parent)?;
    let temporary = parent.join(format!(
        ".{}.{}.tmp",
        path.file_name()
            .and_then(|value| value.to_str())
            .unwrap_or("rc"),
        std::process::id()
    ));
    fs::write(&temporary, value)?;
    fs::rename(&temporary, path)?;
    Ok(())
}

fn validate_token(value: &str, label: &str) -> anyhow::Result<()> {
    anyhow::ensure!(
        !value.is_empty()
            && value.len() <= 96
            && value
                .bytes()
                .all(|byte| { byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'_' | b'.') }),
        "invalid {label} {value:?}"
    );
    Ok(())
}

fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}
