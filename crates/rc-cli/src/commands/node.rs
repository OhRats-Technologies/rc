use super::{encode, env_nonempty};
use crate::{ConfigCommand, DeviceCommand, account};
use anyhow::{Context, Result, bail};
use rc_node::{
    DEFAULT_SERVER, EnrollmentError, NodeConfig, config_path, fetch_status, load_config,
    load_state, resolve_state_dir, save_config, save_state,
};
use std::io;

pub(super) async fn status(url: Option<String>, state_dir: Option<String>) -> Result<()> {
    let dir = resolve_state_dir(state_dir.as_deref());
    let config = load_config(&dir).unwrap_or_default();
    let server = url
        .or_else(|| env_nonempty("RC_URL"))
        .or_else(|| (!config.server.is_empty()).then_some(config.server))
        .unwrap_or_else(|| DEFAULT_SERVER.into());
    println!("RC Node {}", rc_cli::VERSION);
    println!("Config  {}", dir.display());
    println!("RC   {server}");
    let state = match load_state(&dir) {
        Ok(value) => value,
        Err(error) if error.kind() == io::ErrorKind::NotFound => {
            println!("State   not enrolled");
            return Ok(());
        }
        Err(error) => return Err(error.into()),
    };
    println!("Device  {}", state.device_id);
    match fetch_status(&server, &state).await {
        Ok(remote) => {
            println!("Name    {}", remote.name);
            println!("Online  {}", remote.online);
            println!("Agent   {}", remote.version);
        }
        Err(error) => println!("Remote  unavailable ({error})"),
    }
    Ok(())
}

pub(super) async fn devices(url: Option<String>, token: Option<String>) -> Result<()> {
    let client = account::client(url.as_deref(), token.as_deref())?;
    let devices = client.devices().await?;
    if devices.is_empty() {
        println!("No devices");
        return Ok(());
    }
    for device in devices {
        println!(
            "{}  {}  {}  {}  {}",
            device.id,
            device.name,
            device.workspace,
            if device.online { "online" } else { "offline" },
            device.version
        );
    }
    Ok(())
}

pub(super) async fn device(command: DeviceCommand) -> Result<()> {
    match command {
        DeviceCommand::Delete { id, url, token } => {
            let client = account::client(url.as_deref(), token.as_deref())?;
            let devices = client.devices().await?;
            let want = id.trim();
            let matches: Vec<_> = devices
                .into_iter()
                .filter(|device| {
                    device.id == want
                        || device.name.eq_ignore_ascii_case(want)
                        || device.id.starts_with(want)
                })
                .collect();
            let selected = match matches.as_slice() {
                [value] => value,
                [] => bail!("device {id:?} not found"),
                _ => bail!("device {id:?} is ambiguous"),
            };
            let selected_id = selected.id.clone();
            let _: serde_json::Value = client
                .delete(&format!("/api/v1/devices/{}", encode(&selected_id)))
                .await?;
            println!("Removed {selected_id}");
            Ok(())
        }
    }
}

pub(super) fn config(command: Option<ConfigCommand>) -> Result<()> {
    let dir = resolve_state_dir(None);
    let mut value = load_config(&dir).unwrap_or_default();
    match command.unwrap_or(ConfigCommand::Show) {
        ConfigCommand::Show => {
            let server = if value.server.is_empty() {
                DEFAULT_SERVER
            } else {
                &value.server
            };
            let name = if value.name.is_empty() {
                "<hostname>"
            } else {
                &value.name
            };
            println!(
                "server  {server}\nname    {name}\nfile    {}",
                config_path(&dir).display()
            );
        }
        ConfigCommand::Path => println!("{}", config_path(&dir).display()),
        ConfigCommand::Set { key, value: parts } => {
            let input = parts.join(" ").trim().to_owned();
            if input.is_empty() {
                bail!("config value is required");
            }
            set_config(&mut value, &key, input.clone())?;
            save_config(&dir, &value)?;
            println!("{key}  {input}");
        }
        ConfigCommand::Unset { key } => {
            set_config(&mut value, &key, String::new())?;
            save_config(&dir, &value)?;
            println!("unset {key}");
        }
    }
    Ok(())
}

pub(super) async fn enroll(
    token: String,
    url: Option<String>,
    name: Option<String>,
    state_dir: Option<String>,
) -> Result<()> {
    let dir = resolve_state_dir(state_dir.as_deref());
    let mut config = load_config(&dir).unwrap_or_default();
    let server = url
        .clone()
        .or_else(|| env_nonempty("RC_URL"))
        .or_else(|| (!config.server.is_empty()).then_some(config.server.clone()))
        .unwrap_or_else(|| DEFAULT_SERVER.into());
    if let Ok(existing) = load_state(&dir) {
        match fetch_status(&server, &existing).await {
            Ok(remote) => bail!(
                "this OS user is already enrolled as {} ({}). RC keeps one default background enrollment per user; run `rc uninstall` before replacing it, or use `--state-dir` for a separate foreground Node",
                remote.name,
                existing.device_id
            ),
            Err(EnrollmentError::Removed) => bail!(
                "existing enrollment {} was revoked. RC will not overwrite its local identity; run `rc uninstall`, then run the new install/enroll command",
                existing.device_id
            ),
            Err(error) => bail!(
                "could not verify existing enrollment {}: {error}",
                existing.device_id
            ),
        }
    }
    let display_name = name
        .clone()
        .or_else(|| env_nonempty("RC_NAME"))
        .or_else(|| (!config.name.is_empty()).then_some(config.name.clone()))
        .unwrap_or_default();
    if let Some(server_value) = url {
        config.server = server_value.trim_end_matches('/').into();
    }
    if let Some(name_value) = name {
        config.name = name_value;
    }
    if !config.server.is_empty() || !config.name.is_empty() {
        save_config(&dir, &config)?;
    }
    let state = rc_node::enroll(&server, &token, &display_name, rc_cli::VERSION)
        .await
        .context("enrollment failed")?;
    save_state(&dir, &state)?;
    println!("Enrolled {}", state.device_id);
    Ok(())
}

fn set_config(config: &mut NodeConfig, key: &str, value: String) -> Result<()> {
    match key {
        "server" => config.server = value.trim_end_matches('/').into(),
        "name" => config.name = value,
        _ => bail!("unknown config key {key:?}"),
    }
    Ok(())
}
