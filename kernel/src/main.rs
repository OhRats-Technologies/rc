mod bindings;
mod cli;
mod component;
mod config;
mod graph;
mod reconcile;
mod runtime;
mod status;
mod watch;

fn main() -> anyhow::Result<()> {
    cli::run()
}
