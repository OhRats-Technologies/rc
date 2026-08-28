use crate::component_cli::{ComponentCommand, component_commands};
use clap::Command;
use std::{ffi::OsString, fmt::Write as _};

struct Entry {
    name: String,
    summary: String,
}

pub fn top_level_requested(args: &[OsString]) -> bool {
    match args.get(1).and_then(|value| value.to_str()) {
        None => true,
        Some("-h" | "--help" | "help") => args.len() == 2,
        _ => false,
    }
}

pub fn print(command: &Command) {
    print!("{}", render(command));
}

pub fn render(command: &Command) -> String {
    let components = component_commands().unwrap_or_default();
    render_with_components(command, &components)
}

fn render_with_components(command: &Command, components: &[ComponentCommand]) -> String {
    let mut output = format!(
        "RC {}\n\nControl enrolled machines, manage the local RC Node, and extend RC with WebAssembly components.\n\nUsage: rc <command> [options]\n",
        rc_cli::VERSION
    );

    native_group(&mut output, command, "Account", &["login", "logout"]);
    native_group(
        &mut output,
        command,
        "Remote control",
        &["devices", "run", "shell"],
    );
    native_group(
        &mut output,
        command,
        "Node",
        &["enroll", "status", "device", "config", "service"],
    );
    native_group(
        &mut output,
        command,
        "OpenSSH",
        &["ssh-key", "ssh-config", "ssh-proxy"],
    );

    let mut package = component_entries(components, |provider| {
        provider == "ohrats:package-manager" || provider.ends_with("-source")
    });
    package.extend([
        entry("components", "Show the active component graph"),
        entry("commands", "Show commands exported by active components"),
        entry("repair", "Check the trusted component directory"),
    ]);
    group(&mut output, "Components", package);

    group(
        &mut output,
        "Diagnostics",
        component_entries(components, |provider| provider.contains("diagnostics")),
    );

    native_group(
        &mut output,
        command,
        "Platform",
        &["upgrade", "uninstall", "version"],
    );
    group(
        &mut output,
        "Kernel",
        vec![
            entry("backup PATH", "Back up kernel-owned durable state"),
            entry(
                "serve",
                "Serve component HTTP handlers and watch for changes",
            ),
            entry("watch", "Watch and reconcile the component directory"),
        ],
    );

    let extensions = component_entries(components, |provider| {
        provider != "ohrats:package-manager"
            && !provider.ends_with("-source")
            && !provider.contains("diagnostics")
    });
    group(&mut output, "Extensions", extensions);

    output.push_str("\nOptions:\n");
    output.push_str("  -h, --help     Show this help or help for a command\n");
    output.push_str("  -V, --version  Print the installed RC version\n");
    output
}

fn native_group(output: &mut String, command: &Command, title: &str, names: &[&str]) {
    let entries = names
        .iter()
        .filter_map(|name| {
            command
                .get_subcommands()
                .find(|candidate| candidate.get_name() == *name)
                .map(|candidate| Entry {
                    name: (*name).into(),
                    summary: candidate
                        .get_about()
                        .map(ToString::to_string)
                        .unwrap_or_default(),
                })
        })
        .collect();
    group(output, title, entries);
}

fn component_entries(commands: &[ComponentCommand], include: impl Fn(&str) -> bool) -> Vec<Entry> {
    commands
        .iter()
        .filter(|command| include(&command.provider))
        .map(|command| Entry {
            name: command.name.clone(),
            summary: command.summary.clone(),
        })
        .collect()
}

fn entry(name: &str, summary: &str) -> Entry {
    Entry {
        name: name.into(),
        summary: summary.into(),
    }
}

fn group(output: &mut String, title: &str, entries: Vec<Entry>) {
    if entries.is_empty() {
        return;
    }
    let width = entries
        .iter()
        .map(|entry| entry.name.len())
        .max()
        .unwrap_or_default();
    let _ = write!(output, "\n{title}:\n");
    for entry in entries {
        let _ = writeln!(output, "  {:width$}  {}", entry.name, entry.summary);
    }
}

#[cfg(test)]
mod tests {
    use super::{render_with_components, top_level_requested};
    use crate::{Cli, component_cli::ComponentCommand};
    use clap::CommandFactory as _;
    use std::ffi::OsString;

    fn args(values: &[&str]) -> Vec<OsString> {
        values.iter().map(OsString::from).collect()
    }

    #[test]
    fn no_command_and_top_level_help_use_the_grouped_help() {
        assert!(top_level_requested(&args(&["rc"])));
        assert!(top_level_requested(&args(&["rc", "--help"])));
        assert!(top_level_requested(&args(&["rc", "help"])));
        assert!(!top_level_requested(&args(&["rc", "help", "run"])));
        assert!(!top_level_requested(&args(&["rc", "run", "--help"])));
    }

    #[test]
    fn help_groups_native_and_component_commands() {
        let output = render_with_components(
            &Cli::command(),
            &[
                ComponentCommand {
                    name: "update".into(),
                    summary: "Update managed components".into(),
                    provider: "ohrats:package-manager".into(),
                    usage: "rc update [name...]".into(),
                },
                ComponentCommand {
                    name: "doctor".into(),
                    summary: "Show local diagnostics".into(),
                    provider: "ohrats:diagnostics-cli".into(),
                    usage: "rc doctor".into(),
                },
            ],
        );
        for heading in [
            "Account:",
            "Remote control:",
            "Node:",
            "OpenSSH:",
            "Components:",
            "Diagnostics:",
            "Platform:",
            "Kernel:",
        ] {
            assert!(output.contains(heading), "missing {heading}");
        }
        assert!(output.contains("update"));
        assert!(output.contains("doctor"));
        assert!(!output.contains("\nCommands:\n"));
    }
}
