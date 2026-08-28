mod node;
pub(crate) mod node_runtime;
mod remote;
mod ssh;
mod terminal;
mod update;

use crate::{Cli, Command, account};
use anyhow::{Result, bail};
use clap::CommandFactory as _;

pub async fn run(command: Option<Command>) -> Result<()> {
    match command {
        None => {
            crate::help::print(&Cli::command());
            Ok(())
        }
        Some(Command::Version) => {
            println!("RC {}", rc_cli::VERSION);
            Ok(())
        }
        Some(Command::Login { url, expires }) => account::login(url, expires).await,
        Some(Command::Logout) => account::logout().await,
        Some(Command::Status { url, state_dir }) => node::status(url, state_dir).await,
        Some(Command::Devices { url, token }) => node::devices(url, token).await,
        Some(Command::Device { command }) => node::device(command).await,
        Some(Command::Config { command }) => node::config(command),
        Some(Command::Enroll {
            token,
            url,
            name,
            state_dir,
        }) => node::enroll(token, url, name, state_dir).await,
        Some(Command::Run {
            device,
            url,
            token,
            state_dir,
            command,
        }) => {
            if let Some(device) = device {
                if command.is_empty() {
                    bail!("usage: rc run DEVICE -- COMMAND [ARG...]");
                }
                remote::run(device, command, url, token).await
            } else {
                if !command.is_empty() {
                    bail!("remote command requires a DEVICE");
                }
                node_runtime::run(url, state_dir)
            }
        }
        Some(Command::Shell { device, url, token }) => remote::shell(device, url, token).await,
        Some(Command::SshKey { command }) => ssh::key(command).await,
        Some(Command::SshConfig { url, token }) => ssh::config(url, token).await,
        Some(Command::SshProxy { url }) => ssh::proxy(url).await,
        Some(Command::Service { command }) => crate::service::command(command),
        Some(Command::Upgrade) => update::run_upgrade().await,
        Some(Command::Uninstall { url, state_dir }) => update::uninstall(url, state_dir).await,
    }
}

pub(super) fn env_nonempty(key: &str) -> Option<String> {
    std::env::var(key)
        .ok()
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty())
}

pub(super) fn encode(value: &str) -> String {
    url::form_urlencoded::byte_serialize(value.as_bytes()).collect()
}
