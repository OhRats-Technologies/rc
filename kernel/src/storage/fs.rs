use crate::{
    bindings::ohrats::rc_plugin::component_store::InstalledComponent, component::LoadedComponent,
    host::HostEnvironment,
};
use sha2::{Digest, Sha256};
use std::{
    fs,
    path::{Path, PathBuf},
};

pub(crate) const MAX_ARTIFACT_BYTES: usize = 48 * 1024 * 1024;
pub(crate) const MAX_STATE_BYTES: usize = 2 * 1024 * 1024;

pub(crate) fn installed(environment: &HostEnvironment) -> anyhow::Result<Vec<InstalledComponent>> {
    let mut paths = fs::read_dir(environment.component_dir.as_ref())?
        .filter_map(Result::ok)
        .map(|entry| entry.path())
        .filter(|path| {
            path.extension()
                .is_some_and(|extension| extension == "wasm")
        })
        .collect::<Vec<_>>();
    paths.sort();
    paths
        .into_iter()
        .map(|path| {
            let name = path
                .file_stem()
                .and_then(|value| value.to_str())
                .ok_or_else(|| anyhow::anyhow!("invalid component filename {}", path.display()))?;
            let bytes = fs::read(&path)?;
            let (id, version, digest) =
                LoadedComponent::inspect(environment, path.clone(), &bytes)?;
            Ok(InstalledComponent {
                name: name.into(),
                id,
                version: version.to_string(),
                managed: marker_matches(environment, name, &digest),
                digest,
            })
        })
        .collect()
}

pub(crate) fn cache_path(environment: &HostEnvironment, digest: &str) -> anyhow::Result<PathBuf> {
    let value = digest
        .strip_prefix("sha256:")
        .filter(|value| value.len() == 64 && value.bytes().all(|byte| byte.is_ascii_hexdigit()))
        .ok_or_else(|| anyhow::anyhow!("invalid SHA-256 digest {digest:?}"))?;
    Ok(environment
        .cache_dir
        .join("sha256")
        .join(format!("{}.wasm", value.to_ascii_lowercase())))
}

pub(crate) fn verify_digest(expected: &str, artifact: &[u8]) -> anyhow::Result<()> {
    let actual = format!("sha256:{:x}", Sha256::digest(artifact));
    anyhow::ensure!(
        actual.eq_ignore_ascii_case(expected),
        "artifact digest mismatch"
    );
    Ok(())
}

pub(crate) fn marker_path(environment: &HostEnvironment, name: &str) -> PathBuf {
    environment.component_dir.join(format!("{name}.managed"))
}

pub(crate) fn marker_matches(environment: &HostEnvironment, name: &str, digest: &str) -> bool {
    fs::read_to_string(marker_path(environment, name)).is_ok_and(|value| value.trim() == digest)
}

pub(crate) fn component_path(environment: &HostEnvironment, name: &str) -> PathBuf {
    environment.component_dir.join(format!("{name}.wasm"))
}

pub(crate) fn atomic_write(path: &Path, value: &[u8]) -> anyhow::Result<()> {
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

pub(crate) fn validate_token(value: &str, label: &str) -> anyhow::Result<()> {
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
