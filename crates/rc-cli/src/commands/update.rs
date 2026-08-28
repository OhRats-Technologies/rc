use super::env_nonempty;
use anyhow::{Context, Result};
use rc_node::{DEFAULT_SERVER, load_config, load_state, resolve_state_dir};
use std::io;

pub(super) async fn run_upgrade() -> Result<()> {
    let updated = rc_node::replace_executable(rc_cli::VERSION).await?;
    if !updated {
        println!("RC platform {} is already up to date", rc_cli::VERSION);
        return Ok(());
    }
    if crate::service::installed() {
        let dir = resolve_state_dir(None);
        match load_state(&dir) {
            Ok(_) => {
                crate::service::restart().context("upgraded, but could not restart RC Node")?;
                println!("RC platform upgraded and Node restarted");
            }
            Err(error) if error.kind() == io::ErrorKind::NotFound => {
                crate::service::remove()
                    .context("upgraded, but could not remove stale RC Node service")?;
                println!(
                    "RC platform upgraded; removed stale background service because this machine is not enrolled"
                );
            }
            Err(error) => return Err(error.into()),
        }
    } else {
        println!("RC platform upgraded");
    }
    Ok(())
}

pub(super) async fn uninstall(url: Option<String>, state_dir: Option<String>) -> Result<()> {
    let dir = resolve_state_dir(state_dir.as_deref());
    let config = load_config(&dir).unwrap_or_default();
    let server = url
        .or_else(|| env_nonempty("RC_URL"))
        .or_else(|| (!config.server.is_empty()).then_some(config.server))
        .unwrap_or_else(|| DEFAULT_SERVER.into());
    let _ = crate::service::remove();
    if let Ok(state) = load_state(&dir)
        && let Err(error) = rc_node::unregister(&server, &state).await
    {
        eprintln!("warning: server unregister failed: {error}");
    }
    match std::fs::remove_dir_all(&dir) {
        Ok(()) => {}
        Err(error) if error.kind() == io::ErrorKind::NotFound => {}
        Err(error) => return Err(error.into()),
    }
    remove_component_runtime()?;
    if let Ok(path) = std::env::current_exe()
        && path.file_name().and_then(|value| value.to_str()) == Some("rc")
    {
        let _ = std::fs::remove_file(path);
    }
    println!("RC Node uninstalled");
    Ok(())
}

fn remove_component_runtime() -> Result<()> {
    if let Ok(executable) = std::env::current_exe()
        && let Some(parent) = executable.parent()
    {
        match std::fs::remove_file(parent.join("rc-kernel")) {
            Ok(()) => {}
            Err(error) if error.kind() == io::ErrorKind::NotFound => {}
            Err(error) => return Err(error.into()),
        }
    }
    let data = std::env::var_os("RC_DATA_DIR")
        .map(std::path::PathBuf::from)
        .or_else(|| {
            std::env::var_os("HOME")
                .map(std::path::PathBuf::from)
                .map(|home| home.join(".local/share/rc"))
        });
    if let Some(data) = data {
        match std::fs::remove_dir_all(data) {
            Ok(()) => {}
            Err(error) if error.kind() == io::ErrorKind::NotFound => {}
            Err(error) => return Err(error.into()),
        }
    }
    Ok(())
}
