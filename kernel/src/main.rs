mod bindings;
mod cli;
mod component;
mod config;
mod control_primitives;
mod database;
mod descriptor;
mod durable;
mod graph;
mod host;
mod key_vault;
mod network;
mod node;
mod reconcile;
mod runtime;
mod runtime_capabilities;
mod server;
mod service;
mod status;
mod storage;
mod updater;
mod watch;

fn main() -> anyhow::Result<()> {
    #[cfg(windows)]
    if let Some(result) = runtime_capabilities::maybe_run_windows_execution_guard() {
        return result;
    }
    cli::run()
}
