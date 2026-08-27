use crate::{AppState, SshRelay};
use axum::extract::ws::{Message, WebSocket};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_protocol::{ServerToNode, TerminalSpec};
use serde::Deserialize;

const MAX_STDIN_FRAME: usize = 128 * 1024;

#[derive(Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
enum BridgeControl {
    Start {
        command: String,
        terminal: Option<TerminalSpec>,
    },
    StdinClose,
    Resize {
        cols: u16,
        rows: u16,
    },
    Signal {
        signal: String,
    },
}

pub(super) fn parse_start(text: &str) -> Option<(String, Option<TerminalSpec>)> {
    let BridgeControl::Start { command, terminal } = serde_json::from_str(text).ok()? else {
        return None;
    };
    if command.len() > 8192 {
        return None;
    }
    Some((command, terminal))
}

pub(super) async fn forward_relay(socket: &mut WebSocket, relay: Option<SshRelay>) -> bool {
    match relay {
        Some(SshRelay::Stdout(data)) => send_stream(socket, 1, &data).await.is_ok(),
        Some(SshRelay::Stderr(data)) => send_stream(socket, 2, &data).await.is_ok(),
        Some(SshRelay::Exit { code, signal }) => {
            let message = serde_json::json!({
                "type": "exit",
                "code": code,
                "signal": signal,
            });
            let _ = socket.send(Message::Text(message.to_string().into())).await;
            false
        }
        None => false,
    }
}

pub(super) async fn forward_incoming(
    state: &AppState,
    device_id: &str,
    session_id: &str,
    incoming: Option<Result<Message, axum::Error>>,
) -> bool {
    match incoming {
        Some(Ok(Message::Binary(data))) => {
            if data.len() > MAX_STDIN_FRAME {
                tracing::warn!(size = data.len(), "oversized SSH stdin frame");
                return false;
            }
            state
                .nodes
                .send(
                    device_id,
                    &ServerToNode::SshStdin {
                        session_id: session_id.to_owned(),
                        data: URL_SAFE_NO_PAD.encode(data),
                    },
                )
                .await
                .is_ok()
        }
        Some(Ok(Message::Text(text))) => forward_control(state, device_id, session_id, &text).await,
        Some(Ok(Message::Ping(_) | Message::Pong(_))) => true,
        Some(Ok(Message::Close(_))) | Some(Err(_)) | None => false,
    }
}

pub(super) async fn send_exit(
    socket: &mut WebSocket,
    code: i32,
    error: Option<&str>,
) -> Result<(), axum::Error> {
    socket
        .send(Message::Text(
            serde_json::json!({"type": "exit", "code": code, "error": error})
                .to_string()
                .into(),
        ))
        .await
}

async fn send_stream(socket: &mut WebSocket, kind: u8, data: &[u8]) -> Result<(), axum::Error> {
    let mut frame = Vec::with_capacity(data.len() + 1);
    frame.push(kind);
    frame.extend_from_slice(data);
    socket.send(Message::Binary(frame.into())).await
}

async fn forward_control(state: &AppState, device_id: &str, session_id: &str, text: &str) -> bool {
    let Ok(control) = serde_json::from_str::<BridgeControl>(text) else {
        return false;
    };
    let message = match control {
        BridgeControl::StdinClose => ServerToNode::SshStdinClose {
            session_id: session_id.to_owned(),
        },
        BridgeControl::Resize { cols, rows } => ServerToNode::SshResize {
            session_id: session_id.to_owned(),
            cols: cols.clamp(2, 500),
            rows: rows.clamp(2, 500),
        },
        BridgeControl::Signal { signal } => ServerToNode::SshSignal {
            session_id: session_id.to_owned(),
            signal,
        },
        BridgeControl::Start { .. } => return false,
    };
    state.nodes.send(device_id, &message).await.is_ok()
}
