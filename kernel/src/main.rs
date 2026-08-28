mod bindings;
mod cli;
mod component;
mod config;
mod database;
mod descriptor;
mod durable;
mod graph;
mod host;
mod network;
mod node;
mod reconcile;
mod runtime;
mod server;
mod service;
mod status;
mod storage;
mod watch;

fn main() -> anyhow::Result<()> {
    cli::run()
}
