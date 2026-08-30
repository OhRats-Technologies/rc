use anyhow::{Context, Result};
use std::{path::PathBuf, process::Command};

pub(super) fn run(url: Option<String>, state_dir: Option<String>) -> Result<()> {
    let kernel = crate::component_cli::kernel_path()
        .context("RC component kernel is unavailable; run `rc upgrade`")?;
    let state_dir = rc_node::resolve_state_dir(state_dir.as_deref());
    let mut command = Command::new(kernel);
    command
        .arg("node")
        .arg("--state-dir")
        .arg(&state_dir)
        .arg("--agent-version")
        .arg(rc_cli::VERSION);
    if let Some(url) = url {
        command.arg("--server").arg(url);
    }
    exec(command)
}

#[cfg(unix)]
fn exec(mut command: Command) -> Result<()> {
    use std::os::unix::process::CommandExt as _;
    Err(command.exec()).context("could not start RC component Node")
}

#[cfg(not(unix))]
fn exec(mut command: Command) -> Result<()> {
    let status = command.status()?;
    if status.success() {
        Ok(())
    } else {
        anyhow::bail!("RC component Node exited with {status}")
    }
}

pub(crate) fn arguments(state_dir: &std::path::Path) -> Result<(PathBuf, Vec<String>)> {
    let kernel = crate::component_cli::kernel_path()
        .context("RC component kernel is unavailable; run `rc upgrade`")?;
    Ok((
        kernel,
        vec![
            "node".into(),
            "--state-dir".into(),
            state_dir.to_string_lossy().into_owned(),
            "--agent-version".into(),
            rc_cli::VERSION.into(),
        ],
    ))
}
