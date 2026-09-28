use anyhow::{Context as _, Result, bail};
use base64::{Engine as _, engine::general_purpose::STANDARD};
use std::{
    io::Write as _,
    os::windows::process::CommandExt as _,
    path::Path,
    process::{Command, Stdio},
};

const TASK: &str = "OhRats RC Node";
const HOST: &[u8] = include_bytes!(concat!(env!("OUT_DIR"), "/rc-service.exe"));

pub fn install(executable: &Path, arguments: &[String], state_dir: &Path) -> Result<()> {
    rc_platform::protect_private_path(state_dir, true)?;
    let host = state_dir.join("rc-service.exe");
    if std::fs::read(&host).ok().as_deref() != Some(HOST) {
        if installed() {
            stop()?;
        }
        let stage = state_dir.join("rc-service.exe.new");
        std::fs::write(&stage, HOST)?;
        rc_platform::protect_private_path(&stage, false)?;
        for attempt in 0..20 {
            match std::fs::rename(&stage, &host) {
                Ok(()) => break,
                Err(error)
                    if error.kind() == std::io::ErrorKind::PermissionDenied && attempt < 19 =>
                {
                    std::thread::sleep(std::time::Duration::from_millis(100))
                }
                Err(error) => return Err(error.into()),
            }
        }
    }
    let arguments = [
        "--supervise".to_owned(),
        state_dir.to_string_lossy().into_owned(),
        executable.to_string_lossy().into_owned(),
    ]
    .iter()
    .chain(arguments)
    .map(|value| quote(value))
    .collect::<Vec<_>>()
    .join(" ");
    let specification = serde_json::to_vec(&serde_json::json!({
        "executable": host.to_string_lossy(), "arguments": arguments,
    }))?;
    let script = include_str!("windows-install.ps1");
    let encoded = STANDARD.encode(
        script
            .encode_utf16()
            .flat_map(u16::to_le_bytes)
            .collect::<Vec<_>>(),
    );
    let mut child = Command::new("powershell.exe")
        .creation_flags(0x08000000)
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
    let directory = rc_node::resolve_state_dir(None);
    if installed() && directory.is_dir() {
        std::fs::write(directory.join("service.stop"), b"stop")?;
        std::thread::sleep(std::time::Duration::from_secs(1));
    }
    let status = Command::new("schtasks.exe")
        .creation_flags(0x08000000)
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
        .creation_flags(0x08000000)
        .args(["/Query", "/TN", TASK])
        .output()
        .is_ok_and(|output| output.status.success())
}

fn run(arguments: &[&str]) -> Result<()> {
    let status = Command::new("schtasks.exe")
        .creation_flags(0x08000000)
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

    #[test]
    fn windowless_host_logs_and_contains_its_process_tree() -> anyhow::Result<()> {
        let directory = std::env::temp_dir().join(format!(
            "rc-embedded-host-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)?
                .as_nanos()
        ));
        std::fs::create_dir(&directory)?;
        let host = directory.join("rc-service.exe");
        std::fs::write(&host, super::HOST)?;
        let root = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("../..");
        let result = std::process::Command::new("powershell.exe")
            .args([
                "-NoProfile",
                "-NonInteractive",
                "-ExecutionPolicy",
                "Bypass",
                "-File",
                "scripts/windows-service-host-check.ps1",
                "-HostBinary",
            ])
            .arg(&host)
            .current_dir(root)
            .output();
        std::fs::remove_file(host)?;
        std::fs::remove_dir(directory)?;
        let output = result?;
        assert!(
            output.status.success(),
            "{}\n{}",
            String::from_utf8_lossy(&output.stdout),
            String::from_utf8_lossy(&output.stderr)
        );
        Ok(())
    }
}
