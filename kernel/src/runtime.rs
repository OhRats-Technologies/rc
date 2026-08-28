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
    path::{Path, PathBuf},
};

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
        })
    }

    pub fn directory(&self) -> &Path {
        &self.directory
    }

    pub fn reconcile(&mut self) -> anyhow::Result<bool> {
        let paths = graph::component_paths(&self.directory)?;
        let desired = paths.iter().cloned().collect::<BTreeSet<_>>();
        let mut changed =
            reconcile::remove_missing(&mut self.entries, &mut self.failed_paths, &desired);
        self.registry
            .refresh(self.entries.values().map(|entry| &entry.current));
        for path in paths {
            changed |= self.load_path(path);
        }
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

    fn load_path(&mut self, path: PathBuf) -> bool {
        let candidate = match LoadedComponent::load(&self.environment, path.clone()) {
            Ok(value) => value,
            Err(error) => {
                let message = format!("{error:#}");
                let changed = self.failed_paths.get(&path) != Some(&message);
                self.failed_paths.insert(path, message);
                return changed;
            }
        };
        self.failed_paths.remove(&path);
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
            return true;
        };
        if entry.current.path != candidate.path {
            let message = format!(
                "duplicate component id {id:?}; already loaded from {}",
                entry.current.path.display()
            );
            let changed = self.failed_paths.get(&candidate.path) != Some(&message);
            self.failed_paths.insert(candidate.path.clone(), message);
            return changed;
        }
        if entry.current.digest == candidate.digest
            || entry
                .pending
                .as_ref()
                .is_some_and(|pending| pending.digest == candidate.digest)
            || entry.rejected_digest.as_deref() == Some(&candidate.digest)
        {
            return false;
        }
        entry.pending = Some(candidate);
        true
    }
}
