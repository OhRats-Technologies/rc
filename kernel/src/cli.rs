use crate::{config, node, runtime::Runtime, server, watch};
use clap::{Parser, Subcommand};
use std::{io::Write as _, net::SocketAddr, path::PathBuf};

#[derive(Parser)]
#[command(
    name = "rc-kernel",
    disable_help_flag = true,
    disable_help_subcommand = true,
    disable_version_flag = true
)]
struct Arguments {
    #[arg(long, short = 'h')]
    help: bool,

    #[arg(long, short = 'V')]
    version: bool,

    #[arg(long, env = "RC_COMPONENT_DIR")]
    component_dir: Option<PathBuf>,

    #[command(subcommand)]
    command: Option<KernelCommand>,

    #[arg(trailing_var_arg = true, allow_hyphen_values = true)]
    plugin_args: Vec<String>,
}

#[derive(Subcommand)]
enum KernelCommand {
    /// Serve generic component-provided HTTP handlers and watch for changes.
    Serve {
        #[arg(long, env = "RC_LISTEN")]
        listen: Option<SocketAddr>,
    },
    /// Watch the trusted component directory and reconcile changes.
    Watch,
    /// Print the active component graph.
    Components,
    /// Print commands exported by active components.
    Commands,
    /// Check the trusted component directory for failed entries.
    Repair,
    /// Write a consistent online backup of kernel-owned durable state.
    Backup { destination: PathBuf },
    /// Run an enrolled RC Node through component-provided policies.
    Node {
        #[arg(long)]
        state_dir: Option<PathBuf>,
        #[arg(long)]
        server: Option<String>,
        #[arg(long)]
        agent_version: Option<String>,
    },
    #[command(hide = true)]
    PolicyCheck,
    #[command(hide = true)]
    PolicyProbe,
    #[command(hide = true)]
    CryptoCheck,
    #[command(hide = true)]
    CryptoProbe,
    #[command(hide = true)]
    ArgvFixture {
        #[arg(trailing_var_arg = true, allow_hyphen_values = true)]
        values: Vec<String>,
    },
    #[command(hide = true)]
    TextFixture { value: String },
}

pub fn run() -> anyhow::Result<()> {
    let arguments = Arguments::parse();
    if arguments.version {
        println!("RC kernel {}", env!("CARGO_PKG_VERSION"));
        return Ok(());
    }
    if let Some(KernelCommand::ArgvFixture { values }) = &arguments.command {
        let mut output = std::io::stdout().lock();
        for value in values {
            output.write_all(&(value.len() as u64).to_le_bytes())?;
            output.write_all(value.as_bytes())?;
        }
        return Ok(());
    }
    if let Some(KernelCommand::TextFixture { value }) = &arguments.command {
        print!("{value}");
        return Ok(());
    }
    let directory = arguments
        .component_dir
        .unwrap_or_else(config::default_component_dir);
    let mut runtime = Runtime::new(directory)?;
    runtime.reconcile()?;
    if arguments.help {
        print_help(&runtime)?;
        return Ok(());
    }
    match arguments.command {
        Some(KernelCommand::Serve { listen }) => server::run(runtime, listen),
        Some(KernelCommand::Watch) => watch::run(&mut runtime),
        Some(KernelCommand::Components) => {
            print_components(&runtime);
            Ok(())
        }
        Some(KernelCommand::Commands) => {
            print_commands(&runtime)?;
            Ok(())
        }
        Some(KernelCommand::Repair) => repair(&runtime),
        Some(KernelCommand::Backup { destination }) => {
            runtime.backup(&destination)?;
            println!("backup written to {}", destination.display());
            Ok(())
        }
        Some(KernelCommand::Node {
            state_dir,
            server,
            agent_version,
        }) => node::run(
            runtime,
            node::Options {
                state_dir,
                server,
                agent_version,
            },
        ),
        Some(KernelCommand::PolicyCheck) => node::check(&runtime),
        Some(KernelCommand::PolicyProbe) => node::probe(runtime),
        Some(KernelCommand::CryptoCheck) => node::crypto_check(&runtime),
        Some(KernelCommand::CryptoProbe) => node::crypto_probe(runtime),
        Some(KernelCommand::ArgvFixture { .. }) => unreachable!("handled before runtime startup"),
        Some(KernelCommand::TextFixture { .. }) => unreachable!("handled before runtime startup"),
        None => dispatch_plugin(&mut runtime, arguments.plugin_args),
    }
}

fn dispatch_plugin(runtime: &mut Runtime, args: Vec<String>) -> anyhow::Result<()> {
    let Some((command, rest)) = args.split_first() else {
        print_help(runtime)?;
        return Ok(());
    };
    if rest
        .iter()
        .any(|argument| argument == "--help" || argument == "-h")
    {
        let (_, descriptor) = runtime
            .commands()?
            .into_iter()
            .find(|(_, candidate)| candidate.name == *command)
            .ok_or_else(|| anyhow::anyhow!("unknown command {command:?}"))?;
        println!("{}", descriptor.usage);
        println!();
        println!("{}", descriptor.summary);
        return Ok(());
    }
    let code = runtime.invoke(command, rest)?;
    if code != 0 {
        std::process::exit(code as i32);
    }
    Ok(())
}

fn print_components(runtime: &Runtime) {
    println!("ID\tVERSION\tSTATE\tDIGEST\tPATH");
    for status in runtime.statuses() {
        println!(
            "{}\t{}\t{:?}\t{}\t{}{}",
            status.id,
            status.version,
            status.state,
            status.digest,
            status.path.display(),
            status
                .error
                .map(|error| format!("\t{error}"))
                .unwrap_or_default()
        );
    }
}

fn print_commands(runtime: &Runtime) -> anyhow::Result<()> {
    for (provider, command) in runtime.commands()? {
        println!(
            "  {:<18} {:<48} ({provider})",
            command.name, command.summary
        );
        println!("      {}", command.usage);
    }
    Ok(())
}

fn print_help(runtime: &Runtime) -> anyhow::Result<()> {
    println!("RC kernel {}", env!("CARGO_PKG_VERSION"));
    println!();
    println!("Usage: rc-kernel [--component-dir PATH] <command> [args...]");
    println!();
    println!("Kernel commands:");
    println!("  serve              Serve HTTP handlers and reconcile components");
    println!("  watch              Watch and reconcile the component directory");
    println!("  components         Show the component graph");
    println!("  commands           Show active component commands");
    println!("  repair             Diagnose failed components");
    println!("  backup PATH        Back up kernel durable state");
    println!("  node               Run an enrolled Node with component policies");
    let commands = runtime.commands()?;
    if !commands.is_empty() {
        println!();
        println!("Component commands:");
        print_commands(runtime)?;
    }
    Ok(())
}

fn repair(runtime: &Runtime) -> anyhow::Result<()> {
    runtime.integrity_check()?;
    let failed = runtime
        .statuses()
        .into_iter()
        .filter(|status| status.error.is_some())
        .collect::<Vec<_>>();
    if failed.is_empty() {
        println!("component directory is healthy");
        return Ok(());
    }
    for status in &failed {
        eprintln!(
            "{}: {}",
            status.path.display(),
            status.error.expect("failed status has no error")
        );
    }
    anyhow::bail!("{} component(s) require attention", failed.len())
}
