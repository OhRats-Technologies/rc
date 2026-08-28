use anyhow::{Context, Result};
use std::{
    ffi::OsString,
    path::PathBuf,
    process::{Command, Stdio},
};

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ComponentCommand {
    pub name: String,
    pub summary: String,
    pub provider: String,
    pub usage: String,
}

const KERNEL_COMMANDS: &[&str] = &[
    "backup",
    "commands",
    "components",
    "repair",
    "serve",
    "watch",
];

const NATIVE_COMMANDS: &[&str] = &[
    "config",
    "device",
    "devices",
    "enroll",
    "login",
    "logout",
    "run",
    "service",
    "shell",
    "ssh-config",
    "ssh-key",
    "ssh-proxy",
    "status",
    "uninstall",
    "upgrade",
    "version",
];

pub fn dispatch_if_component_command() -> Result<Option<i32>> {
    let args = std::env::args_os().skip(1).collect::<Vec<_>>();
    let Some(first) = args.first().and_then(|value| value.to_str()) else {
        return Ok(None);
    };
    if first.starts_with('-') || NATIVE_COMMANDS.contains(&first) {
        return Ok(None);
    }
    let Some(kernel) = kernel_path() else {
        return Ok(None);
    };
    if first == "help"
        && let Some(command) = args.get(1).and_then(|value| value.to_str())
        && (KERNEL_COMMANDS.contains(&command) || component_command_exists(&kernel, command)?)
    {
        return run_kernel(&kernel, [OsString::from(command), OsString::from("--help")]);
    }
    if !KERNEL_COMMANDS.contains(&first) && !component_command_exists(&kernel, first)? {
        return Ok(None);
    }
    run_kernel(&kernel, args)
}

fn run_kernel(kernel: &PathBuf, args: impl IntoIterator<Item = OsString>) -> Result<Option<i32>> {
    let status = Command::new(kernel).args(args).status().with_context(|| {
        format!(
            "could not start RC component kernel at {}",
            kernel.display()
        )
    })?;
    Ok(Some(status.code().unwrap_or(1)))
}

fn component_command_exists(kernel: &PathBuf, command: &str) -> Result<bool> {
    Ok(component_commands_from(kernel)?
        .iter()
        .any(|candidate| candidate.name == command))
}

pub fn component_commands() -> Result<Vec<ComponentCommand>> {
    let Some(kernel) = kernel_path() else {
        return Ok(Vec::new());
    };
    component_commands_from(&kernel)
}

fn component_commands_from(kernel: &PathBuf) -> Result<Vec<ComponentCommand>> {
    let output = Command::new(kernel)
        .arg("commands")
        .stdin(Stdio::null())
        .stderr(Stdio::null())
        .output();
    let output = match output {
        Ok(output) if output.status.success() => output,
        Ok(_) => return Ok(Vec::new()),
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(Vec::new()),
        Err(error) => return Err(error.into()),
    };
    Ok(parse_component_commands(&String::from_utf8_lossy(
        &output.stdout,
    )))
}

fn parse_component_commands(listing: &str) -> Vec<ComponentCommand> {
    let mut commands: Vec<ComponentCommand> = Vec::new();
    for line in listing.lines() {
        if !line.starts_with("  ") || line.starts_with("      ") {
            if let Some(command) = commands.last_mut()
                && let Some(usage) = line.trim().strip_prefix("rc ")
            {
                command.usage = format!("rc {usage}");
            }
            continue;
        }
        let line = line.trim();
        let Some(provider_start) = line.rfind("(") else {
            continue;
        };
        let Some(provider) = line[provider_start..]
            .strip_prefix('(')
            .and_then(|value| value.strip_suffix(')'))
        else {
            continue;
        };
        let description = line[..provider_start].trim_end();
        let Some((name, summary)) = description.split_once(char::is_whitespace) else {
            continue;
        };
        commands.push(ComponentCommand {
            name: name.into(),
            summary: summary.trim().into(),
            provider: provider.into(),
            usage: format!("rc {name} --help"),
        });
    }
    commands
}

pub(crate) fn kernel_path() -> Option<PathBuf> {
    if let Some(value) = std::env::var_os("RC_KERNEL").filter(|value| !value.is_empty()) {
        return Some(PathBuf::from(value));
    }
    if let Ok(current) = std::env::current_exe()
        && let Some(parent) = current.parent()
    {
        let sibling = parent.join("rc-kernel");
        if sibling.is_file() {
            return Some(sibling);
        }
    }
    path_lookup("rc-kernel")
}

fn path_lookup(name: &str) -> Option<PathBuf> {
    let path = std::env::var_os("PATH")?;
    std::env::split_paths(&path)
        .map(|directory| directory.join(OsString::from(name)))
        .find(|candidate| candidate.is_file())
}

#[cfg(test)]
mod tests {
    use super::{ComponentCommand, KERNEL_COMMANDS, NATIVE_COMMANDS, parse_component_commands};

    #[test]
    fn component_and_native_command_names_do_not_overlap() {
        for command in KERNEL_COMMANDS {
            assert!(!NATIVE_COMMANDS.contains(command));
        }
        assert!(!NATIVE_COMMANDS.contains(&"update"));
    }

    #[test]
    fn parses_component_command_catalog() {
        assert_eq!(
            parse_component_commands(
                "  update             Update managed components                    (ohrats:package-manager)\n      rc update [name...] [--latest]\n"
            ),
            vec![ComponentCommand {
                name: "update".into(),
                summary: "Update managed components".into(),
                provider: "ohrats:package-manager".into(),
                usage: "rc update [name...] [--latest]".into(),
            }]
        );
    }
}
