use crate::bindings::ohrats::rc_plugin::types::{Command, Requirement, Service};
use crate::bindings::{Descriptor, Plugin};
use anyhow::Context as _;
use semver::{Version, VersionReq};
use sha2::{Digest, Sha256};
use std::{collections::BTreeSet, fs, path::PathBuf};
use wasmtime::{Config, Engine, Store, StoreLimits, StoreLimitsBuilder};
use wasmtime_wasi::{ResourceTable, WasiCtx, WasiCtxBuilder, WasiCtxView, WasiView};

const ACTIVATION_FUEL: u64 = 5_000_000;
const INVOCATION_FUEL: u64 = 50_000_000;

pub struct HostState {
    table: ResourceTable,
    wasi: WasiCtx,
    limits: StoreLimits,
    plugin_id: String,
}

impl HostState {
    fn new(plugin_id: String) -> Self {
        let mut wasi = WasiCtxBuilder::new();
        wasi.inherit_stdout().inherit_stderr();
        Self {
            table: ResourceTable::new(),
            wasi: wasi.build(),
            limits: StoreLimitsBuilder::new()
                .memory_size(64 * 1024 * 1024)
                .table_elements(100_000)
                .instances(128)
                .memories(16)
                .tables(32)
                .trap_on_grow_failure(true)
                .build(),
            plugin_id,
        }
    }
}

impl WasiView for HostState {
    fn ctx(&mut self) -> WasiCtxView<'_> {
        WasiCtxView {
            ctx: &mut self.wasi,
            table: &mut self.table,
        }
    }
}

impl crate::bindings::ohrats::rc_plugin::host::Host for HostState {
    fn log(&mut self, level: crate::bindings::ohrats::rc_plugin::host::LogLevel, message: String) {
        eprintln!("[{}] {level:?}: {message}", self.plugin_id);
    }
}

#[derive(Debug, Clone)]
pub struct ValidatedService {
    pub name: String,
    pub version: Version,
    pub priority: i32,
}

#[derive(Debug, Clone)]
pub struct ValidatedRequirement {
    pub name: String,
    pub version: VersionReq,
}

#[derive(Debug, Clone)]
pub struct ValidatedCommand {
    pub name: String,
    pub summary: String,
    pub usage: String,
}

#[derive(Debug, Clone)]
pub struct ValidatedDescriptor {
    pub id: String,
    pub version: Version,
    pub provides: Vec<ValidatedService>,
    pub requires: Vec<ValidatedRequirement>,
    pub commands: Vec<ValidatedCommand>,
}

pub struct LoadedComponent {
    pub path: PathBuf,
    pub digest: String,
    pub descriptor: ValidatedDescriptor,
    store: Store<HostState>,
    bindings: Plugin,
    active: bool,
}

impl LoadedComponent {
    pub fn load(engine: &Engine, path: PathBuf) -> anyhow::Result<Self> {
        let bytes = fs::read(&path)
            .with_context(|| format!("failed to read component {}", path.display()))?;
        let digest = format!("sha256:{:x}", Sha256::digest(&bytes));
        let component = wasmtime::component::Component::new(engine, &bytes).map_err(|error| {
            anyhow::anyhow!("failed to compile component {}: {error}", path.display())
        })?;
        let mut linker = wasmtime::component::Linker::new(engine);
        wasmtime_wasi::p2::add_to_linker_sync(&mut linker)?;
        crate::bindings::ohrats::rc_plugin::host::add_to_linker::<
            HostState,
            wasmtime::component::HasSelf<HostState>,
        >(&mut linker, |state| state)?;
        let mut store = Store::new(engine, HostState::new(path.display().to_string()));
        store.limiter(|state| &mut state.limits);
        store.set_hostcall_fuel(1024 * 1024);
        store.set_fuel(INVOCATION_FUEL)?;
        let bindings = Plugin::instantiate(&mut store, &component, &linker).map_err(|error| {
            anyhow::anyhow!(
                "failed to instantiate component {}: {error}",
                path.display()
            )
        })?;
        let descriptor = bindings.call_descriptor(&mut store)?;
        let descriptor = validate_descriptor(descriptor)?;
        store.data_mut().plugin_id = descriptor.id.clone();
        Ok(Self {
            path,
            digest,
            descriptor,
            store,
            bindings,
            active: false,
        })
    }

    pub fn activate(&mut self) -> anyhow::Result<()> {
        self.store.set_fuel(ACTIVATION_FUEL)?;
        match self.bindings.call_activate(&mut self.store)? {
            Ok(()) => {
                self.active = true;
                Ok(())
            }
            Err(error) => anyhow::bail!(error),
        }
    }

    pub fn deactivate(&mut self) {
        if self.active {
            let _ = self.store.set_fuel(ACTIVATION_FUEL);
            let _ = self.bindings.call_deactivate(&mut self.store);
            self.active = false;
        }
    }

    pub fn invoke(&mut self, command: &str, args: &[String]) -> anyhow::Result<u32> {
        self.store.set_fuel(INVOCATION_FUEL)?;
        match self.bindings.call_invoke(&mut self.store, command, args)? {
            Ok(code) => Ok(code),
            Err(error) => anyhow::bail!(error),
        }
    }

    pub fn is_active(&self) -> bool {
        self.active
    }
}

impl Drop for LoadedComponent {
    fn drop(&mut self) {
        self.deactivate();
    }
}

pub fn engine() -> anyhow::Result<Engine> {
    let mut config = Config::new();
    config.wasm_component_model(true);
    config.consume_fuel(true);
    Ok(Engine::new(&config)?)
}

fn validate_descriptor(value: Descriptor) -> anyhow::Result<ValidatedDescriptor> {
    validate_name(&value.id, "component id")?;
    let version = Version::parse(&value.version).context("invalid component version")?;
    let provides = value
        .provides
        .into_iter()
        .map(validate_service)
        .collect::<anyhow::Result<Vec<_>>>()?;
    let requires = value
        .requires
        .into_iter()
        .map(validate_requirement)
        .collect::<anyhow::Result<Vec<_>>>()?;
    let commands = value
        .commands
        .into_iter()
        .map(validate_command)
        .collect::<anyhow::Result<Vec<_>>>()?;
    ensure_unique(
        provides.iter().map(|service| service.name.as_str()),
        "service",
    )?;
    ensure_unique(
        commands.iter().map(|command| command.name.as_str()),
        "command",
    )?;
    Ok(ValidatedDescriptor {
        id: value.id,
        version,
        provides,
        requires,
        commands,
    })
}

fn validate_service(value: Service) -> anyhow::Result<ValidatedService> {
    validate_name(&value.name, "service name")?;
    Ok(ValidatedService {
        name: value.name,
        version: Version::parse(&value.version).context("invalid service version")?,
        priority: value.priority,
    })
}

fn validate_requirement(value: Requirement) -> anyhow::Result<ValidatedRequirement> {
    validate_name(&value.name, "requirement name")?;
    Ok(ValidatedRequirement {
        name: value.name,
        version: VersionReq::parse(&value.version).context("invalid service requirement")?,
    })
}

fn validate_command(value: Command) -> anyhow::Result<ValidatedCommand> {
    validate_command_name(&value.name)?;
    anyhow::ensure!(!value.summary.trim().is_empty(), "command summary is empty");
    anyhow::ensure!(!value.usage.trim().is_empty(), "command usage is empty");
    Ok(ValidatedCommand {
        name: value.name,
        summary: value.summary,
        usage: value.usage,
    })
}

fn validate_name(value: &str, label: &str) -> anyhow::Result<()> {
    anyhow::ensure!(
        !value.is_empty()
            && value.len() <= 128
            && value.bytes().all(|byte| {
                byte.is_ascii_alphanumeric() || matches!(byte, b':' | b'/' | b'.' | b'-' | b'_')
            }),
        "invalid {label} {value:?}"
    );
    Ok(())
}

fn validate_command_name(value: &str) -> anyhow::Result<()> {
    anyhow::ensure!(
        !value.is_empty()
            && value.len() <= 64
            && value
                .bytes()
                .all(|byte| { byte.is_ascii_lowercase() || byte.is_ascii_digit() || byte == b'-' }),
        "invalid command name {value:?}"
    );
    Ok(())
}

fn ensure_unique<'a>(values: impl IntoIterator<Item = &'a str>, label: &str) -> anyhow::Result<()> {
    let mut seen = BTreeSet::new();
    for value in values {
        anyhow::ensure!(seen.insert(value), "duplicate {label} {value:?}");
    }
    Ok(())
}
