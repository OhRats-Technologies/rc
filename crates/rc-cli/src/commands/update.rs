use super::env_nonempty;
use anyhow::{Context, Result};
use rc_node::{DEFAULT_SERVER, load_config, load_state, resolve_state_dir};
use std::io;

pub(super) async fn run_update() -> Result<()> {
    let updated = rc_node::replace_executable(rc_cli::VERSION).await?;
    if !updated {
        println!("RC {} is already up to date", rc_cli::VERSION);
        return Ok(());
    }
    if crate::service::installed() {
        let dir = resolve_state_dir(None);
        match load_state(&dir) {
            Ok(_) => {
                crate::service::restart().context("updated, but could not restart RC Node")?;
                println!("RC Node updated and restarted");
            }
            Err(error) if error.kind() == io::ErrorKind::NotFound => {
                crate::service::remove()
                    .context("updated, but could not remove stale RC Node service")?;
                println!(
                    "RC Node updated; removed stale background service because this machine is not enrolled"
                );
            }
            Err(error) => return Err(error.into()),
        }
    } else {
        println!("RC Node updated");
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
    if let Ok(path) = std::env::current_exe()
        && path.file_name().and_then(|value| value.to_str()) == Some("rc")
    {
        let _ = std::fs::remove_file(path);
    }
    println!("RC Node uninstalled");
    Ok(())
}
