mod bindings;
mod cli;
mod component;
mod config;
mod descriptor;
mod graph;
mod host;
mod reconcile;
mod runtime;
mod service;
mod status;
mod storage;
mod watch;

fn main() -> anyhow::Result<()> {
    cli::run()
}
