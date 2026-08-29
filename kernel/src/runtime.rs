use crate::component::{LoadedComponent, ValidatedCommand};
use crate::graph;
use crate::host::{HostEnvironment, engine};
use crate::reconcile;
use crate::service::ServiceRegistry;
use crate::status::{ComponentState, ComponentStatus};
use anyhow::Context as _;
use std::{
    collections::{BTreeMap, BTreeSet},
    fs,
    io::ErrorKind,
    path::{Path, PathBuf},
    thread,
    time::Duration,
};

const RECONCILE_ATTEMPTS: usize = 4;
const RECONCILE_BACKOFF: Duration = Duration::from_millis(10);

struct LoadOutcome {
    changed: bool,
    vanished: bool,
}

pub(crate) struct Entry {
    pub(crate) current: LoadedComponent,
    pub(crate) pending: Option<LoadedComponent>,
    pub(crate) error: Option<String>,
    pub(crate) rejected_digest: Option<String>,
}

pub struct Runtime {
    environment: HostEnvironment,
    directory: PathBuf,
    entries: BTreeMap<String, Entry>,
    failed_paths: BTreeMap<PathBuf, String>,
    registry: ServiceRegistry,
    #[cfg(test)]
    after_scan: Option<Box<dyn FnOnce() + Send>>,
}

impl Runtime {
    pub fn new(directory: PathBuf) -> anyhow::Result<Self> {
        fs::create_dir_all(&directory)
            .with_context(|| format!("failed to create {}", directory.display()))?;
        let engine = engine()?;
        let environment = HostEnvironment::new(engine.clone(), directory.clone())?;
        Ok(Self {
            environment,
            directory,
            entries: BTreeMap::new(),
            failed_paths: BTreeMap::new(),
            registry: ServiceRegistry::default(),
            #[cfg(test)]
            after_scan: None,
        })
    }

    pub fn directory(&self) -> &Path {
        &self.directory
    }

    pub fn reconcile(&mut self) -> anyhow::Result<bool> {
        let mut changed = false;
        for attempt in 0..RECONCILE_ATTEMPTS {
            let paths = graph::component_paths(&self.directory)?;
            let desired = paths.iter().cloned().collect::<BTreeSet<_>>();
            #[cfg(test)]
            if let Some(after_scan) = self.after_scan.take() {
                after_scan();
            }
            self.registry
                .refresh(self.entries.values().map(|entry| &entry.current));
            let mut vanished = false;
            for path in paths {
                let outcome = self.load_path(path);
                changed |= outcome.changed;
                vanished |= outcome.vanished;
            }
            let confirmed = graph::component_paths(&self.directory)?
                .into_iter()
                .collect::<BTreeSet<_>>();
            if !vanished && desired == confirmed {
                changed |= reconcile::remove_missing(
                    &mut self.entries,
                    &mut self.failed_paths,
                    &confirmed,
                );
                changed |= reconcile::states(&mut self.entries, &self.registry);
                return Ok(changed);
            }
            if attempt + 1 < RECONCILE_ATTEMPTS {
                thread::sleep(RECONCILE_BACKOFF);
            }
        }
        // Continuous churn exhausted the bounded attempts. Preserve the last
        // healthy generations; a later notification can start a fresh pass.
        changed |= reconcile::states(&mut self.entries, &self.registry);
        Ok(changed)
    }

    pub fn statuses(&self) -> Vec<ComponentStatus<'_>> {
        let mut values = self
            .entries
            .values()
            .map(|entry| ComponentStatus {
                id: &entry.current.descriptor.id,
                version: entry.current.descriptor.version.to_string(),
                digest: &entry.current.digest,
                path: &entry.current.path,
                state: if entry.current.is_active() {
                    ComponentState::Active
                } else if entry.error.is_some() {
                    ComponentState::Failed
                } else {
                    ComponentState::Waiting
                },
                error: entry.error.as_deref(),
            })
            .collect::<Vec<_>>();
        for (path, error) in &self.failed_paths {
            values.push(ComponentStatus {
                id: "<invalid>",
                version: "-".into(),
                digest: "-",
                path,
                state: ComponentState::Failed,
                error: Some(error),
            });
        }
        values.sort_by(|left, right| left.id.cmp(right.id));
        values
    }

    pub fn service_registry(&self) -> ServiceRegistry {
        self.registry.clone()
    }

    pub fn integrity_check(&self) -> anyhow::Result<()> {
        self.environment.database.integrity_check()
    }

    pub fn backup(&self, destination: &Path) -> anyhow::Result<()> {
        self.environment.database.backup_to(destination)
    }

    pub fn commands(&self) -> anyhow::Result<Vec<(&str, &ValidatedCommand)>> {
        let mut commands = BTreeMap::new();
        for entry in self.entries.values() {
            if !entry.current.is_active() {
                continue;
            }
            for command in &entry.current.descriptor.commands {
                if let Some((previous, _)) =
                    commands.insert(command.name.as_str(), (entry, command))
                {
                    anyhow::bail!(
                        "command {:?} is provided by both {} and {}",
                        command.name,
                        previous.current.descriptor.id,
                        entry.current.descriptor.id
                    );
                }
            }
        }
        Ok(commands
            .into_values()
            .map(|(entry, command)| (entry.current.descriptor.id.as_str(), command))
            .collect())
    }

    pub fn invoke(&mut self, command: &str, args: &[String]) -> anyhow::Result<u32> {
        let ids = self
            .entries
            .iter()
            .filter(|(_, entry)| {
                entry.current.is_active()
                    && entry
                        .current
                        .descriptor
                        .commands
                        .iter()
                        .any(|candidate| candidate.name == command)
            })
            .map(|(id, _)| id.clone())
            .collect::<Vec<_>>();
        anyhow::ensure!(!ids.is_empty(), "unknown command {command:?}");
        anyhow::ensure!(ids.len() == 1, "ambiguous command {command:?}");
        self.entries
            .get_mut(&ids[0])
            .expect("component disappeared")
            .current
            .invoke(command, args)
    }

    fn load_path(&mut self, path: PathBuf) -> LoadOutcome {
        let candidate = match LoadedComponent::load(&self.environment, path.clone()) {
            Ok(value) => value,
            Err(error) => {
                if error
                    .downcast_ref::<std::io::Error>()
                    .is_some_and(|error| error.kind() == ErrorKind::NotFound)
                {
                    let changed = self.failed_paths.remove(&path).is_some();
                    return LoadOutcome {
                        changed,
                        vanished: true,
                    };
                }
                let message = format!("{error:#}");
                let changed = self.failed_paths.get(&path) != Some(&message);
                self.failed_paths.insert(path, message);
                return LoadOutcome {
                    changed,
                    vanished: false,
                };
            }
        };
        let cleared_path_failure = self.failed_paths.remove(&path).is_some();
        let id = candidate.descriptor.id.clone();
        let Some(entry) = self.entries.get_mut(&id) else {
            self.entries.insert(
                id,
                Entry {
                    current: candidate,
                    pending: None,
                    error: None,
                    rejected_digest: None,
                },
            );
            return LoadOutcome {
                changed: true,
                vanished: false,
            };
        };
        if entry.current.path != candidate.path {
            let message = format!(
                "duplicate component id {id:?}; already loaded from {}",
                entry.current.path.display()
            );
            let changed = self.failed_paths.get(&candidate.path) != Some(&message);
            self.failed_paths.insert(candidate.path.clone(), message);
            return LoadOutcome {
                changed,
                vanished: false,
            };
        }
        if entry.current.digest == candidate.digest
            || entry
                .pending
                .as_ref()
                .is_some_and(|pending| pending.digest == candidate.digest)
            || entry.rejected_digest.as_deref() == Some(&candidate.digest)
        {
            let restored_current = entry.current.digest == candidate.digest
                && entry.current.is_active()
                && (entry.error.is_some() || entry.rejected_digest.is_some());
            if restored_current {
                entry.error = None;
                entry.rejected_digest = None;
            }
            return LoadOutcome {
                changed: cleared_path_failure || restored_current,
                vanished: false,
            };
        }
        entry.pending = Some(candidate);
        LoadOutcome {
            changed: true,
            vanished: false,
        }
    }
}

#[cfg(test)]
#[path = "runtime/tests.rs"]
mod tests;
