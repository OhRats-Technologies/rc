#[cfg(windows)]
use super::terminal::next_terminal_resize;
use super::{
    encode,
    terminal::{
        RawTerminal, read_shell_output, terminal_attached, terminal_size, wait_remote_process,
    },
};
use crate::{account, control_client::RemoteControl};
use anyhow::{Result, bail};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_api_client::{ApiClient, Credential, Device};
use rc_node::resolve_state_dir;
use rc_protocol::{ControlMessage, EnvironmentSpec, ExecutionMode, TerminalSpec};
use tokio::io::AsyncReadExt;

pub(super) async fn run(
    selector: String,
    mut command: Vec<String>,
    url: Option<String>,
    token: Option<String>,
) -> Result<()> {
    let (client, credential, device) =
        remote_device(url.as_deref(), token.as_deref(), &selector).await?;
    let process_id = allocate_process(&client, &device.id, false).await?;
    let dir = resolve_state_dir(None);
    let mut control = RemoteControl::open(client.clone(), &credential, &device, &dir).await?;
    let program = command.remove(0);
    control
        .sender
        .send(&ControlMessage::ProcessStart {
            id: process_id.clone(),
            mode: ExecutionMode::Argv {
                program,
                args: command,
            },
            cwd: None,
            environment: EnvironmentSpec::default(),
            terminal: None,
        })
        .await?;
    eprintln!("Started {process_id} on {}", device.name);
    let result = wait_remote_process(&control.sender, &mut control.receiver, &process_id).await;
    control.close().await;
    result
}

pub(super) async fn run_shell(
    selector: String,
    script: String,
    url: Option<String>,
    token: Option<String>,
) -> Result<()> {
    if script.trim().is_empty() {
        bail!("portable RC Shell source must not be empty");
    }
    let (client, credential, device) =
        remote_device(url.as_deref(), token.as_deref(), &selector).await?;
    let process_id = allocate_process(&client, &device.id, false).await?;
    let dir = resolve_state_dir(None);
    let mut control = RemoteControl::open(client.clone(), &credential, &device, &dir).await?;
    control
        .sender
        .send(&ControlMessage::ProcessStart {
            id: process_id.clone(),
            mode: ExecutionMode::RcShell { script },
            cwd: None,
            environment: EnvironmentSpec::default(),
            terminal: None,
        })
        .await?;
    eprintln!("Started {process_id} on {}", device.name);
    let result = wait_remote_process(&control.sender, &mut control.receiver, &process_id).await;
    control.close().await;
    result
}

pub(super) async fn shell(
    selector: String,
    url: Option<String>,
    token: Option<String>,
) -> Result<()> {
    if !terminal_attached() {
        bail!("shell requires an interactive terminal");
    }
    let (client, credential, device) =
        remote_device(url.as_deref(), token.as_deref(), &selector).await?;
    let process_id = allocate_process(&client, &device.id, true).await?;
    let dir = resolve_state_dir(None);
    let mut control = RemoteControl::open(client.clone(), &credential, &device, &dir).await?;
    let (cols, rows) = terminal_size();
    let _raw = RawTerminal::enter()?;
    control
        .sender
        .send(&ControlMessage::ProcessStart {
            id: process_id.clone(),
            mode: ExecutionMode::SystemLoginShell,
            cwd: None,
            environment: EnvironmentSpec::default(),
            terminal: Some(TerminalSpec {
                cols,
                rows,
                term: std::env::var("TERM").unwrap_or_else(|_| "xterm-256color".into()),
            }),
        })
        .await?;
    let input_sender = control.sender.clone();
    let input_id = process_id.clone();
    let input = tokio::spawn(async move {
        let mut stdin = tokio::io::stdin();
        let mut buffer = [0_u8; 4096];
        loop {
            match stdin.read(&mut buffer).await {
                Ok(0) => {
                    let _ = input_sender
                        .send(&ControlMessage::ProcessStdinClose {
                            id: input_id.clone(),
                        })
                        .await;
                    break;
                }
                Ok(count) => {
                    if input_sender
                        .send(&ControlMessage::ProcessStdin {
                            id: input_id.clone(),
                            data: URL_SAFE_NO_PAD.encode(&buffer[..count]),
                        })
                        .await
                        .is_err()
                    {
                        break;
                    }
                }
                Err(_) => break,
            }
        }
    });
    #[cfg(unix)]
    let resize = {
        let sender = control.sender.clone();
        let id = process_id.clone();
        tokio::spawn(async move {
            let Ok(mut signal) =
                tokio::signal::unix::signal(tokio::signal::unix::SignalKind::window_change())
            else {
                return;
            };
            while signal.recv().await.is_some() {
                let (cols, rows) = terminal_size();
                let _ = sender
                    .send(&ControlMessage::ProcessResize {
                        id: id.clone(),
                        cols,
                        rows,
                    })
                    .await;
            }
        })
    };
    #[cfg(windows)]
    let resize = {
        let sender = control.sender.clone();
        let id = process_id.clone();
        tokio::spawn(async move {
            let mut previous = (cols, rows);
            loop {
                let (cols, rows) = next_terminal_resize(previous).await;
                previous = (cols, rows);
                if sender
                    .send(&ControlMessage::ProcessResize {
                        id: id.clone(),
                        cols,
                        rows,
                    })
                    .await
                    .is_err()
                {
                    return;
                }
            }
        })
    };
    let result = read_shell_output(&mut control.receiver, &process_id).await;
    input.abort();
    #[cfg(any(unix, windows))]
    resize.abort();
    control.close().await;
    result
}

pub(super) async fn remote_device(
    url: Option<&str>,
    token: Option<&str>,
    selector: &str,
) -> Result<(ApiClient, Credential, Device)> {
    let (server, credential) = account::defaults(url, token)?;
    let client = ApiClient::new(&server, credential.clone())?;
    let devices = client.devices().await?;
    let want = selector.trim();
    let matches: Vec<_> = devices
        .into_iter()
        .filter(|device| {
            device.id == want
                || device.name.eq_ignore_ascii_case(want)
                || device.id.starts_with(want)
        })
        .collect();
    let device = match matches.as_slice() {
        [device] => client.device(&device.id).await?,
        [] => bail!("device {selector:?} not found"),
        _ => bail!("device {selector:?} is ambiguous"),
    };
    if !device.online {
        bail!("device {} is offline", device.name);
    }
    Ok((client, credential, device))
}

async fn allocate_process(client: &ApiClient, device_id: &str, terminal: bool) -> Result<String> {
    #[derive(serde::Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct Created {
        process_id: String,
    }
    let value: Created = client
        .post(
            &format!("/api/v1/devices/{}/processes", encode(device_id)),
            &serde_json::json!({"terminal":terminal}),
        )
        .await?;
    if value.process_id.is_empty() {
        bail!("RC server did not allocate a process");
    }
    Ok(value.process_id)
}
