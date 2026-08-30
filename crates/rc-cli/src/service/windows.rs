use anyhow::{Context as _, Result, bail};
use std::{path::Path, process::Command};

const TASK: &str = "OhRats RC Node";

pub fn install(executable: &Path, arguments: &[String]) -> Result<()> {
    let command = std::iter::once(executable.to_string_lossy().into_owned())
        .chain(arguments.iter().cloned())
        .map(|value| quote(&value))
        .collect::<Vec<_>>()
        .join(" ");
    run(&[
        "/Create", "/F", "/SC", "ONLOGON", "/RL", "LIMITED", "/IT", "/TN", TASK, "/TR", &command,
    ])?;
    run(&["/Run", "/TN", TASK])
}

pub fn stop() -> Result<()> {
    let status = Command::new("schtasks.exe")
        .args(["/End", "/TN", TASK])
        .status()
        .context("could not stop the RC scheduled task")?;
    if status.success() || !installed() {
        Ok(())
    } else {
        bail!("schtasks.exe exited with {status}")
    }
}

pub fn status() -> Result<()> {
    run(&["/Query", "/TN", TASK, "/V", "/FO", "LIST"])
}

pub fn remove() -> Result<()> {
    if installed() {
        run(&["/Delete", "/F", "/TN", TASK])?;
    }
    Ok(())
}

pub fn installed() -> bool {
    Command::new("schtasks.exe")
        .args(["/Query", "/TN", TASK])
        .output()
        .is_ok_and(|output| output.status.success())
}

fn run(arguments: &[&str]) -> Result<()> {
    let status = Command::new("schtasks.exe")
        .args(arguments)
        .status()
        .context("could not invoke Windows Task Scheduler")?;
    if status.success() {
        Ok(())
    } else {
        bail!("schtasks.exe exited with {status}")
    }
}

fn quote(value: &str) -> String {
    let mut output = String::from('"');
    let mut backslashes = 0;
    for character in value.chars() {
        match character {
            '\\' => backslashes += 1,
            '"' => {
                output.push_str(&"\\".repeat(backslashes * 2 + 1));
                output.push('"');
                backslashes = 0;
            }
            _ => {
                output.push_str(&"\\".repeat(backslashes));
                backslashes = 0;
                output.push(character);
            }
        }
    }
    output.push_str(&"\\".repeat(backslashes * 2));
    output.push('"');
    output
}

#[cfg(test)]
mod tests {
    use super::quote;

    #[test]
    fn command_line_quoting_preserves_spaces_quotes_and_trailing_slashes() {
        assert_eq!(quote("hello world"), "\"hello world\"");
        assert_eq!(quote("a\"b"), "\"a\\\"b\"");
        assert_eq!(quote("C:\\path\\"), "\"C:\\path\\\\\"");
    }
}
