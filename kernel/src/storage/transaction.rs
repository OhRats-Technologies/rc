use super::fs as storage_fs;
mod lock;
mod metadata;
mod recovery;
use crate::{
    bindings::ohrats::rc_plugin::component_store::{
        InstalledComponent, PreparedComponent, PreparedSet, UpdateCandidate,
    },
    component::LoadedComponent,
    host::HostEnvironment,
};
use sha2::{Digest, Sha256};
use std::{
    collections::BTreeSet,
    fs,
    path::{Path, PathBuf},
    sync::atomic::{AtomicU64, Ordering},
};

const MAX_CANDIDATES: usize = 128;
const MAX_CANDIDATE_BYTES: usize = 256 * 1024 * 1024;
const TRANSACTIONS: &str = "component-transactions";
static NEXT_ID: AtomicU64 = AtomicU64::new(1);

pub(crate) struct Candidate {
    pub(crate) name: String,
    pub(crate) artifact: Vec<u8>,
    pub(crate) expected_digest: Option<String>,
}

impl From<UpdateCandidate> for Candidate {
    fn from(value: UpdateCandidate) -> Self {
        Self {
            name: value.name,
            artifact: value.artifact,
            expected_digest: value.expected_digest,
        }
    }
}

pub(crate) struct Prepared {
    id: String,
    components: Vec<PreparedComponent>,
}

impl From<Prepared> for PreparedSet {
    fn from(value: Prepared) -> Self {
        Self {
            id: value.id,
            components: value.components,
        }
    }
}

pub(crate) fn prepare(
    environment: &HostEnvironment,
    candidates: &[Candidate],
) -> anyhow::Result<Prepared> {
    recover(environment)?;
    anyhow::ensure!(
        !candidates.is_empty(),
        "update transaction has no candidates"
    );
    anyhow::ensure!(
        candidates.len() <= MAX_CANDIDATES,
        "update transaction has too many candidates"
    );
    let total_bytes = candidates.iter().try_fold(0usize, |total, candidate| {
        total
            .checked_add(candidate.artifact.len())
            .ok_or_else(|| anyhow::anyhow!("candidate size overflow"))
    })?;
    anyhow::ensure!(
        total_bytes <= MAX_CANDIDATE_BYTES,
        "update transaction exceeds 256 MiB"
    );
    let id = format!(
        "{}-{}",
        std::process::id(),
        NEXT_ID.fetch_add(1, Ordering::Relaxed)
    );
    let directory = transaction_path(environment, &id)?;
    fs::create_dir_all(&directory)?;
    let result = (|| {
        let mut names = BTreeSet::new();
        let mut components = Vec::with_capacity(candidates.len());
        for candidate in candidates {
            storage_fs::validate_token(&candidate.name, "component name")?;
            anyhow::ensure!(
                names.insert(candidate.name.clone()),
                "duplicate component {:?}",
                candidate.name
            );
            anyhow::ensure!(
                candidate.artifact.len() <= storage_fs::MAX_ARTIFACT_BYTES,
                "artifact exceeds 48 MiB"
            );
            let digest = format!("sha256:{:x}", Sha256::digest(&candidate.artifact));
            if let Some(expected) = candidate.expected_digest.as_deref() {
                anyhow::ensure!(
                    expected.eq_ignore_ascii_case(&digest),
                    "artifact digest does not match expected digest"
                );
            }
            let component_path = storage_fs::component_path(environment, &candidate.name);
            anyhow::ensure!(
                !component_path.exists()
                    || storage_fs::marker_path(environment, &candidate.name).is_file(),
                "refusing to replace unmanaged component {:?}",
                candidate.name
            );
            let (id, version, inspected_digest) =
                LoadedComponent::inspect(environment, component_path, &candidate.artifact)?;
            anyhow::ensure!(
                inspected_digest == digest,
                "artifact digest changed during inspection"
            );
            let cache = storage_fs::cache_path(environment, &digest)?;
            storage_fs::atomic_write(&cache, &candidate.artifact)?;
            storage_fs::atomic_write(
                &directory.join(format!("{}.wasm", candidate.name)),
                &candidate.artifact,
            )?;
            components.push(PreparedComponent {
                name: candidate.name.clone(),
                id,
                version: version.to_string(),
                digest,
            });
        }
        write_names(
            &directory.join("manifest"),
            components.iter().map(|value| value.name.as_str()),
        )?;
        metadata::write_metadata(&directory.join("metadata"), &id, &components)?;
        storage_fs::atomic_write(&directory.join("phase"), b"prepared\n")?;
        Ok(Prepared { id, components })
    })();
    if result.is_err() {
        let _ = fs::remove_dir_all(&directory);
    }
    result
}

pub(crate) fn commit(
    environment: &HostEnvironment,
    prepared: &PreparedSet,
) -> anyhow::Result<Vec<InstalledComponent>> {
    let _publication = lock::Publication::acquire(environment)?;
    let prepared = PreparedRef::try_from(prepared)?;
    let directory = transaction_path(environment, &prepared.id)?;
    metadata::verify_manifest(&directory, &prepared.id, &prepared.components)?;
    let mut changed = Vec::new();
    let backup = directory.join("backup");
    fs::create_dir_all(&backup)?;
    let result = (|| {
        for value in &prepared.components {
            let staged = directory.join(format!("{}.wasm", value.name));
            let bytes = fs::read(&staged)?;
            storage_fs::verify_digest(&value.digest, &bytes)?;
            let path = storage_fs::component_path(environment, &value.name);
            let marker = storage_fs::marker_path(environment, &value.name);
            if path.exists() {
                anyhow::ensure!(
                    marker.is_file(),
                    "refusing to replace unmanaged component {:?}",
                    value.name
                );
                let current = fs::read(&path)?;
                let current_digest = format!("sha256:{:x}", Sha256::digest(&current));
                if current_digest.eq_ignore_ascii_case(&value.digest) {
                    continue;
                }
                storage_fs::atomic_write(&backup.join(format!("{}.wasm", value.name)), &current)?;
                storage_fs::atomic_write(
                    &backup.join(format!("{}.managed", value.name)),
                    &fs::read(&marker)?,
                )?;
            } else {
                anyhow::ensure!(
                    !marker.exists(),
                    "stale managed marker for {:?}",
                    value.name
                );
            }
            fs::write(
                temp_path(environment, &prepared.id, &value.name, "wasm"),
                &bytes,
            )?;
            fs::write(
                temp_path(environment, &prepared.id, &value.name, "managed"),
                format!("{}\n", value.digest),
            )?;
            changed.push(value.name.clone());
        }
        write_names(
            &directory.join("changed"),
            changed.iter().map(String::as_str),
        )?;
        storage_fs::atomic_write(&directory.join("phase"), b"committing\n")?;
        for name in &changed {
            fs::rename(
                temp_path(environment, &prepared.id, name, "wasm"),
                storage_fs::component_path(environment, name),
            )?;
            fs::rename(
                temp_path(environment, &prepared.id, name, "managed"),
                storage_fs::marker_path(environment, name),
            )?;
        }
        storage_fs::atomic_write(&directory.join("phase"), b"committed\n")?;
        Ok(prepared
            .components
            .iter()
            .map(|value| InstalledComponent {
                name: value.name.clone(),
                id: value.id.clone(),
                version: value.version.clone(),
                digest: value.digest.clone(),
                managed: true,
            })
            .collect())
    })();
    if let Err(error) = result {
        recovery::rollback(environment, &directory, &changed)?;
        let _ = fs::remove_dir_all(&directory);
        return Err(error);
    }
    let _ = fs::remove_dir_all(&directory);
    result
}

pub(crate) fn abort(environment: &HostEnvironment, prepared: &PreparedSet) {
    let Ok(_publication) = lock::Publication::acquire(environment) else {
        return;
    };
    let Ok(prepared) = PreparedRef::try_from(prepared) else {
        return;
    };
    let Ok(directory) = transaction_path(environment, &prepared.id) else {
        return;
    };
    let phase = recovery::read_phase(&directory).unwrap_or_default();
    if phase == "committing" {
        let changed = recovery::read_names(&directory.join("changed")).unwrap_or_default();
        let _ = recovery::rollback(environment, &directory, &changed);
    }
    let _ = fs::remove_dir_all(directory);
}

pub(crate) fn recover(environment: &HostEnvironment) -> anyhow::Result<()> {
    let _recovery = lock::Recovery::acquire(environment)?;
    recovery::recover(environment)
}

struct PreparedRef {
    id: String,
    components: Vec<PreparedComponent>,
}

impl TryFrom<&PreparedSet> for PreparedRef {
    type Error = anyhow::Error;
    fn try_from(value: &PreparedSet) -> Result<Self, Self::Error> {
        storage_fs::validate_token(&value.id, "transaction id")?;
        anyhow::ensure!(
            !value.components.is_empty() && value.components.len() <= MAX_CANDIDATES,
            "invalid prepared update"
        );
        let mut names = BTreeSet::new();
        for component in &value.components {
            storage_fs::validate_token(&component.name, "component name")?;
            anyhow::ensure!(
                names.insert(component.name.clone()),
                "duplicate prepared component {:?}",
                component.name
            );
        }
        Ok(Self {
            id: value.id.clone(),
            components: value.components.clone(),
        })
    }
}

fn transaction_path(environment: &HostEnvironment, id: &str) -> anyhow::Result<PathBuf> {
    storage_fs::validate_token(id, "transaction id")?;
    Ok(environment.cache_dir.join(TRANSACTIONS).join(id))
}

fn temp_path(environment: &HostEnvironment, id: &str, name: &str, suffix: &str) -> PathBuf {
    environment
        .component_dir
        .join(format!(".rc-update-{id}-{name}.{suffix}.tmp"))
}

fn write_names<'a>(path: &Path, names: impl IntoIterator<Item = &'a str>) -> anyhow::Result<()> {
    let text = names.into_iter().collect::<Vec<_>>().join("\n");
    storage_fs::atomic_write(path, format!("{text}\n").as_bytes())
}
