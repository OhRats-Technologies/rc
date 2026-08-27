use futures_util::{SinkExt, StreamExt};
use serde::Deserialize;
use std::{env, process::ExitCode};
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio_tungstenite::tungstenite::Message;

#[tokio::main]
async fn main() -> ExitCode {
    match run().await {
        Ok(code) => ExitCode::from(code),
        Err(error) => {
            eprintln!("{error}");
            ExitCode::from(255)
        }
    }
}

async fn run() -> anyhow::Result<u8> {
    let args: Vec<_> = env::args().collect();
    match args.get(1).map(String::as_str) {
        Some("authorized") => authorized(args.get(2), args.get(3)).await,
        Some("bridge") => bridge(args.get(2)).await,
        _ => anyhow::bail!("usage: rc-ssh-helper authorized TYPE KEY | bridge KEY_ID"),
    }
}

async fn authorized(kind: Option<&String>, key: Option<&String>) -> anyhow::Result<u8> {
    let (kind, key) = (
        kind.ok_or_else(|| anyhow::anyhow!("missing key type"))?,
        key.ok_or_else(|| anyhow::anyhow!("missing key"))?,
    );
    let port = env::var("RC_SSH_INTERNAL_PORT").unwrap_or_else(|_| "3001".into());
    let url = format!(
        "http://127.0.0.1:{port}/authorized?type={}&key={}",
        url::form_urlencoded::byte_serialize(kind.as_bytes()).collect::<String>(),
        url::form_urlencoded::byte_serialize(key.as_bytes()).collect::<String>()
    );
    let response = reqwest::get(url).await?;
    if !response.status().is_success() {
        return Ok(1);
    };
    print!("{}", response.text().await?);
    Ok(0)
}

async fn bridge(key_id: Option<&String>) -> anyhow::Result<u8> {
    let key_id = key_id.ok_or_else(|| anyhow::anyhow!("RC SSH key missing"))?;
    let device = env::var("RC_DEVICE_ID").unwrap_or_default();
    if device.is_empty() {
        anyhow::bail!("RC SSH target missing; regenerate your RC SSH config.")
    }
    let port = env::var("RC_SSH_INTERNAL_PORT").unwrap_or_else(|_| "3001".into());
    let url = format!(
        "ws://127.0.0.1:{port}/bridge?keyId={}&deviceId={}",
        enc(key_id),
        enc(&device)
    );
    let (socket, _) = tokio_tungstenite::connect_async(url).await?;
    let (mut send, mut receive) = socket.split();
    let terminal = is_terminal();
    let (cols, rows) = terminal_size();
    let terminal_spec = terminal.then(|| {
        serde_json::json!({
            "cols": cols,
            "rows": rows,
            "term": env::var("TERM").unwrap_or_else(|_| "xterm-256color".into()),
        })
    });
    let start = serde_json::json!({
        "type": "start",
        "command": env::var("SSH_ORIGINAL_COMMAND").unwrap_or_default(),
        "terminal": terminal_spec,
    });
    send.send(Message::Text(start.to_string().into())).await?;
    let (mut stdin, mut stdout, mut stderr) =
        (tokio::io::stdin(), tokio::io::stdout(), tokio::io::stderr());
    let mut input = vec![0u8; 32 * 1024];
    let mut exit = 255u8;
    let mut stdin_open = true;
    loop {
        tokio::select! {
            read = stdin.read(&mut input), if stdin_open => {
                let count = read?;
                if let Some(message) = stdin_message(&mut stdin_open, &input[..count]) {
                    send.send(message).await?;
                }
            }
            message = receive.next() => {
                match message {
                    Some(Ok(Message::Binary(data))) => {
                        write_stream_frame(&data, &mut stdout, &mut stderr).await?;
                    }
                    Some(Ok(Message::Text(text))) => {
                        let message: ExitMessage = serde_json::from_str(&text)?;
                        if message.r#type != "exit" {
                            anyhow::bail!("unexpected RC SSH control message");
                        }
                        if let Some(error) = message.error.filter(|value| !value.is_empty()) {
                            eprintln!("{error}");
                        }
                        exit = message.code.unwrap_or(255).clamp(0, 255) as u8;
                        break;
                    }
                    Some(Ok(Message::Ping(data))) => {
                        send.send(Message::Pong(data)).await?;
                    }
                    Some(Ok(Message::Pong(_))) => {}
                    Some(Ok(Message::Close(_))) | Some(Err(_)) | None => break,
                    Some(Ok(Message::Frame(_))) => {}
                }
            }
            _ = resize_signal(), if terminal => {
                let (cols, rows) = terminal_size();
                let resize = serde_json::json!({
                    "type": "resize",
                    "cols": cols,
                    "rows": rows,
                });
                if send.send(Message::Text(resize.to_string().into())).await.is_err() {
                    break;
                }
            }
        }
    }
    Ok(exit)
}

#[derive(Deserialize)]
struct ExitMessage {
    r#type: String,
    code: Option<i64>,
    error: Option<String>,
}

fn stdin_message(open: &mut bool, data: &[u8]) -> Option<Message> {
    if !*open {
        return None;
    }
    if data.is_empty() {
        *open = false;
        return Some(Message::Text(
            serde_json::json!({"type": "stdin_close"})
                .to_string()
                .into(),
        ));
    }
    Some(Message::Binary(data.to_vec().into()))
}

async fn write_stream_frame(
    data: &[u8],
    stdout: &mut tokio::io::Stdout,
    stderr: &mut tokio::io::Stderr,
) -> anyhow::Result<()> {
    let Some((&stream, payload)) = data.split_first() else {
        anyhow::bail!("empty RC SSH stream frame");
    };
    match stream {
        1 => {
            stdout.write_all(payload).await?;
            stdout.flush().await?;
        }
        2 => {
            stderr.write_all(payload).await?;
            stderr.flush().await?;
        }
        _ => anyhow::bail!("invalid RC SSH stream identifier"),
    }
    Ok(())
}

fn enc(value: &str) -> String {
    url::form_urlencoded::byte_serialize(value.as_bytes()).collect()
}

fn is_terminal() -> bool {
    // SAFETY: isatty only reads the validity and terminal status of these process-owned FDs.
    unsafe { libc::isatty(libc::STDIN_FILENO) == 1 && libc::isatty(libc::STDOUT_FILENO) == 1 }
}

fn terminal_size() -> (u16, u16) {
    let mut size = libc::winsize {
        ws_row: 24,
        ws_col: 80,
        ws_xpixel: 0,
        ws_ypixel: 0,
    };
    // SAFETY: `size` is a valid writable winsize for the duration of this ioctl call.
    unsafe { libc::ioctl(libc::STDOUT_FILENO, libc::TIOCGWINSZ, &mut size) };
    (size.ws_col.clamp(2, 500), size.ws_row.clamp(2, 500))
}
#[cfg(unix)]
async fn resize_signal() {
    if let Ok(mut signal) =
        tokio::signal::unix::signal(tokio::signal::unix::SignalKind::window_change())
    {
        signal.recv().await;
    } else {
        std::future::pending::<()>().await
    }
}
#[cfg(not(unix))]
async fn resize_signal() {
    std::future::pending::<()>().await
}

#[cfg(test)]
mod tests {
    use super::stdin_message;
    use tokio_tungstenite::tungstenite::Message;

    #[test]
    fn stdin_eof_is_forwarded_only_once() {
        let mut open = true;
        assert!(matches!(
            stdin_message(&mut open, &[]),
            Some(Message::Text(_))
        ));
        assert!(!open);
        assert!(stdin_message(&mut open, &[]).is_none());
    }
}
