use crate::{
    bindings::Plugin,
    descriptor::{self, ValidatedDescriptor},
    host::{self, ACTIVATION_FUEL, HostEnvironment},
    service::{self, InstanceHandle, ServiceRegistry},
};
use anyhow::Context as _;
use sha2::{Digest, Sha256};
use std::{fs, path::PathBuf};
use wasmtime::component::Component;

pub use crate::descriptor::ValidatedCommand;

pub struct LoadedComponent {
    pub path: PathBuf,
    pub digest: String,
    pub descriptor: ValidatedDescriptor,
    component: Component,
    environment: HostEnvironment,
    active: Option<InstanceHandle>,
}

impl LoadedComponent {
    pub fn load(environment: &HostEnvironment, path: PathBuf) -> anyhow::Result<Self> {
        let bytes = fs::read(&path)
            .with_context(|| format!("failed to read component {}", path.display()))?;
        Self::from_bytes(environment, path, &bytes)
    }

    pub fn inspect(
        environment: &HostEnvironment,
        label: PathBuf,
        bytes: &[u8],
    ) -> anyhow::Result<(String, semver::Version, String)> {
        let value = Self::from_bytes(environment, label, bytes)?;
        Ok((
            value.descriptor.id.clone(),
            value.descriptor.version.clone(),
            value.digest.clone(),
        ))
    }

    fn from_bytes(
        environment: &HostEnvironment,
        path: PathBuf,
        bytes: &[u8],
    ) -> anyhow::Result<Self> {
        let engine = &environment.engine;
        let digest = format!("sha256:{:x}", Sha256::digest(bytes));
        let component = Component::new(engine, bytes).map_err(|error| {
            anyhow::anyhow!("failed to compile component {}: {error}", path.display())
        })?;
        let mut store = host::store(
            environment,
            path.display().to_string(),
            ServiceRegistry::default(),
        )?;
        let linker = service::linker(
            engine,
            &component,
            &empty_descriptor(),
            &ServiceRegistry::default(),
            true,
        )?;
        let instance = linker.instantiate(&mut store, &component)?;
        let bindings = Plugin::new(&mut store, &instance)?;
        let descriptor = bindings.call_descriptor(&mut store)?;
        let descriptor = descriptor::validate(descriptor, &component, engine)?;
        store.data_mut().set_plugin_id(descriptor.id.clone());
        Ok(Self {
            path,
            digest,
            descriptor,
            component,
            environment: environment.clone(),
            active: None,
        })
    }

    pub fn activate(&mut self, registry: &ServiceRegistry) -> anyhow::Result<()> {
        let engine = self.component.engine();
        let linker = service::linker(engine, &self.component, &self.descriptor, registry, false)?;
        let mut store = host::store(
            &self.environment,
            self.descriptor.id.clone(),
            registry.clone(),
        )?;
        let instance = linker.instantiate(&mut store, &self.component)?;
        let bindings = Plugin::new(&mut store, &instance)?;
        store.set_fuel(ACTIVATION_FUEL)?;
        match bindings.call_activate(&mut store)? {
            Ok(()) => {
                let active = service::active_instance(
                    &self.component,
                    &instance,
                    store,
                    bindings,
                    &self.descriptor,
                )?;
                self.active = Some(service::Generation::new(active));
                Ok(())
            }
            Err(error) => anyhow::bail!(error),
        }
    }

    pub fn deactivate(&mut self) {
        let Some(active) = self.active.take() else {
            return;
        };
        active.deactivate();
    }

    pub fn invoke(&mut self, command: &str, args: &[String]) -> anyhow::Result<u32> {
        let active = self
            .active
            .as_ref()
            .context("component is not active")?
            .clone();
        active.invoke(command, args)
    }

    pub fn is_active(&self) -> bool {
        self.active
            .as_ref()
            .is_some_and(|active| active.is_available())
    }

    pub(crate) fn withdraw(&self) {
        if let Some(active) = &self.active {
            active.withdraw();
        }
    }

    pub fn active_handle(&self) -> Option<InstanceHandle> {
        self.active.clone()
    }
}

impl Drop for LoadedComponent {
    fn drop(&mut self) {
        self.deactivate();
    }
}

fn empty_descriptor() -> ValidatedDescriptor {
    ValidatedDescriptor {
        id: "metadata".into(),
        version: semver::Version::new(0, 0, 0),
        provides: Vec::new(),
        requires: Vec::new(),
        commands: Vec::new(),
    }
}
