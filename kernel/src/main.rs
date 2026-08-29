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
mod server;
mod service;
mod status;
mod storage;
mod updater;
mod watch;

fn main() -> anyhow::Result<()> {
    cli::run()
}
