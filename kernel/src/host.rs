use crate::bindings::ohrats::rc_plugin::host::{Host, LogLevel};
use std::{path::PathBuf, sync::Arc};
use wasmtime::{Config, Engine, Store, StoreLimits, StoreLimitsBuilder};
use wasmtime_wasi::{ResourceTable, WasiCtx, WasiCtxBuilder, WasiCtxView, WasiView};

pub const ACTIVATION_FUEL: u64 = 5_000_000;
pub const INVOCATION_FUEL: u64 = 50_000_000;
pub const SERVICE_FUEL: u64 = 50_000_000;

#[derive(Clone)]
pub struct HostEnvironment {
    pub engine: Engine,
    pub component_dir: Arc<PathBuf>,
    pub state_dir: Arc<PathBuf>,
}

impl HostEnvironment {
    pub fn new(engine: Engine, component_dir: PathBuf) -> anyhow::Result<Self> {
        let root = component_dir
            .parent()
            .map(PathBuf::from)
            .unwrap_or_else(|| component_dir.clone());
        let state_dir = root.join("state");
        std::fs::create_dir_all(&component_dir)?;
        std::fs::create_dir_all(&state_dir)?;
        Ok(Self {
            engine,
            component_dir: Arc::new(component_dir),
            state_dir: Arc::new(state_dir),
        })
    }
}

pub struct HostState {
    table: ResourceTable,
    wasi: WasiCtx,
    limits: StoreLimits,
    plugin_id: String,
    pub environment: HostEnvironment,
}

impl HostState {
    pub fn new(environment: HostEnvironment, plugin_id: String) -> Self {
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
            environment,
        }
    }

    pub fn plugin_id(&self) -> &str {
        &self.plugin_id
    }

    pub fn set_plugin_id(&mut self, plugin_id: String) {
        self.plugin_id = plugin_id;
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

impl Host for HostState {
    fn log(&mut self, level: LogLevel, message: String) {
        eprintln!("[{}] {level:?}: {message}", self.plugin_id);
    }
}

pub fn engine() -> anyhow::Result<Engine> {
    let mut config = Config::new();
    config.wasm_component_model(true);
    config.consume_fuel(true);
    Ok(Engine::new(&config)?)
}

pub fn store(environment: &HostEnvironment, plugin_id: String) -> anyhow::Result<Store<HostState>> {
    let mut store = Store::new(
        &environment.engine,
        HostState::new(environment.clone(), plugin_id),
    );
    store.limiter(|state| &mut state.limits);
    store.set_hostcall_fuel(128 * 1024 * 1024);
    store.set_fuel(INVOCATION_FUEL)?;
    Ok(store)
}

pub fn add_base_imports(linker: &mut wasmtime::component::Linker<HostState>) -> anyhow::Result<()> {
    wasmtime_wasi::p2::add_to_linker_sync(linker)?;
    crate::bindings::ohrats::rc_plugin::host::add_to_linker::<
        HostState,
        wasmtime::component::HasSelf<HostState>,
    >(linker, |state| state)?;
    crate::bindings::ohrats::rc_plugin::component_store::add_to_linker::<
        HostState,
        wasmtime::component::HasSelf<HostState>,
    >(linker, |state| state)?;
    crate::bindings::ohrats::rc_plugin::state_store::add_to_linker::<
        HostState,
        wasmtime::component::HasSelf<HostState>,
    >(linker, |state| state)?;
    crate::bindings::ohrats::rc_plugin::local_files::add_to_linker::<
        HostState,
        wasmtime::component::HasSelf<HostState>,
    >(linker, |state| state)?;
    Ok(())
}
