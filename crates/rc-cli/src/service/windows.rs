use anyhow::{Context as _, Result, bail};
use base64::{Engine as _, engine::general_purpose::STANDARD};
use std::{
    io::Write as _,
    path::Path,
    process::{Command, Stdio},
};

const TASK: &str = "OhRats RC Node";

pub fn install(executable: &Path, arguments: &[String]) -> Result<()> {
    let arguments = arguments
        .iter()
        .map(|value| quote(value))
        .collect::<Vec<_>>()
        .join(" ");
    let specification = serde_json::to_vec(&serde_json::json!({
        "executable": executable.to_string_lossy(), "arguments": arguments,
    }))?;
    let script = include_str!("windows-install.ps1");
    let encoded = STANDARD.encode(
        script
            .encode_utf16()
            .flat_map(u16::to_le_bytes)
            .collect::<Vec<_>>(),
    );
    let mut child = Command::new("powershell.exe")
        .args([
            "-NoLogo",
            "-NoProfile",
            "-NonInteractive",
            "-EncodedCommand",
            &encoded,
        ])
        .stdin(Stdio::piped())
        .spawn()
        .context("could not register the per-user RC scheduled task")?;
    let input_result = child
        .stdin
        .take()
        .context("service registration stdin unavailable")?
        .write_all(&specification);
    let status = child.wait()?;
    input_result?;
    if !status.success() {
        bail!("per-user RC service registration failed with {status}");
    }
    Ok(())
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
