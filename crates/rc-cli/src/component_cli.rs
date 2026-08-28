use anyhow::{Context, Result};
use std::{
    ffi::OsString,
    path::PathBuf,
    process::{Command, Stdio},
};

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
    if !KERNEL_COMMANDS.contains(&first) && !component_command_exists(&kernel, first)? {
        return Ok(None);
    }
    let status = Command::new(&kernel)
        .args(&args)
        .status()
        .with_context(|| {
            format!(
                "could not start RC component kernel at {}",
                kernel.display()
            )
        })?;
    Ok(Some(status.code().unwrap_or(1)))
}

fn component_command_exists(kernel: &PathBuf, command: &str) -> Result<bool> {
    let output = Command::new(kernel)
        .arg("commands")
        .stdin(Stdio::null())
        .stderr(Stdio::null())
        .output();
    let output = match output {
        Ok(output) if output.status.success() => output,
        Ok(_) => return Ok(false),
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(false),
        Err(error) => return Err(error.into()),
    };
    let listing = String::from_utf8_lossy(&output.stdout);
    Ok(listing
        .lines()
        .any(|line| line.starts_with("  ") && line.split_whitespace().next() == Some(command)))
}

fn kernel_path() -> Option<PathBuf> {
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
    use super::{KERNEL_COMMANDS, NATIVE_COMMANDS};

    #[test]
    fn component_and_native_command_names_do_not_overlap() {
        for command in KERNEL_COMMANDS {
            assert!(!NATIVE_COMMANDS.contains(command));
        }
        assert!(!NATIVE_COMMANDS.contains(&"update"));
    }
}
