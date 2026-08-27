mod io;

use crate::{AppState, now_ms};
use axum::{
    extract::{Query, State, WebSocketUpgrade, ws::WebSocket},
    http::StatusCode,
    response::{IntoResponse, Response},
};
use futures_util::SinkExt;
use io::{forward_incoming, forward_relay, parse_start, send_exit};
use rc_protocol::{ControlProof, ServerToNode};
use rusqlite::OptionalExtension;
use serde::Deserialize;
use std::time::Duration;
use uuid::Uuid;

const SFTP_COMMAND: &str = r#"if command -v sftp-server >/dev/null 2>&1; then exec "$(command -v sftp-server)"; elif [ -x /usr/lib/openssh/sftp-server ]; then exec /usr/lib/openssh/sftp-server; elif [ -x /usr/lib/ssh/sftp-server ]; then exec /usr/lib/ssh/sftp-server; elif [ -x /usr/libexec/sftp-server ]; then exec /usr/libexec/sftp-server; else echo 'sftp-server not installed' >&2; exit 127; fi"#;

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub(super) struct BridgeQuery {
    key_id: String,
    device_id: String,
}

pub(super) async fn bridge(
    State(state): State<AppState>,
    Query(query): Query<BridgeQuery>,
    upgrade: WebSocketUpgrade,
) -> Response {
    let principal = match ssh_principal(&state, &query.key_id, &query.device_id) {
        Ok(Some(principal)) => principal,
        Ok(None) => return StatusCode::FORBIDDEN.into_response(),
        Err(error) => {
            tracing::error!(%error, "SSH bridge authorization lookup failed");
            return StatusCode::INTERNAL_SERVER_ERROR.into_response();
        }
    };
    upgrade
        .on_upgrade(move |socket| bridge_socket(state, principal, socket))
        .into_response()
}

struct SshPrincipal {
    key_id: String,
    user_id: String,
    device_id: String,
    proof: ControlProof,
}

fn ssh_principal(
    state: &AppState,
    key_id: &str,
    device_id: &str,
) -> anyhow::Result<Option<SshPrincipal>> {
    Ok(state.db.with_connection(|db| {
        db.query_row(
            "SELECT k.id,k.user_id,c.grant,c.credential_id,c.assertion \
             FROM ssh_keys k \
             JOIN clients c ON c.id=k.client_id \
             JOIN devices d ON d.id=? \
             JOIN workspace_members wm \
               ON wm.workspace_id=d.workspace_id AND wm.user_id=k.user_id \
             WHERE k.id=? AND wm.role IN ('owner','operator') \
               AND (c.expires_at=0 OR c.expires_at>?)",
            rusqlite::params![device_id, key_id, now_ms()],
            |row| {
                Ok(SshPrincipal {
                    key_id: row.get(0)?,
                    user_id: row.get(1)?,
                    device_id: device_id.to_owned(),
                    proof: ControlProof {
                        grant: row.get(2)?,
                        credential_id: row.get(3)?,
                        assertion: row.get(4)?,
                    },
                })
            },
        )
        .optional()
    })?)
}

async fn bridge_socket(state: AppState, principal: SshPrincipal, mut socket: WebSocket) {
    let Some((command, terminal)) = receive_start(&mut socket).await else {
        let _ = socket.close().await;
        return;
    };
    if !state.nodes.online(&principal.device_id).await {
        let _ = send_exit(&mut socket, 255, Some("RC Node is offline")).await;
        let _ = socket.close().await;
        return;
    }

    let process_id = Uuid::new_v4().to_string();
    let session_id = Uuid::new_v4().to_string();
    let command = shell_command(&command);
    let mut receiver = state
        .ssh
        .register(&session_id, &principal.device_id, &process_id);
    if let Err(error) = insert_process(&state, &principal, &process_id, terminal.is_some()) {
        tracing::error!(%error, %process_id, "failed to create SSH process");
        state.ssh.remove(&session_id);
        let _ = send_exit(&mut socket, 255, Some("RC server error")).await;
        let _ = socket.close().await;
        return;
    }

    let start = ServerToNode::SshStart {
        process_id: process_id.clone(),
        session_id: session_id.clone(),
        user_id: principal.user_id.clone(),
        command,
        cwd: String::new(),
        terminal,
        grant: principal.proof.grant.clone(),
        credential_id: principal.proof.credential_id.clone(),
        assertion: principal.proof.assertion.clone(),
    };
    if state
        .nodes
        .send(&principal.device_id, &start)
        .await
        .is_err()
    {
        state.ssh.remove(&session_id);
        mark_process_lost(&state, &process_id, "RC Node disconnected before SSH start");
        let _ = send_exit(&mut socket, 255, Some("RC Node is offline")).await;
        let _ = socket.close().await;
        return;
    }
    touch_key(&state, &principal.key_id);

    loop {
        tokio::select! {
            relay = receiver.recv() => {
                if !forward_relay(&mut socket, relay).await {
                    break;
                }
            }
            incoming = socket.recv() => {
                if !forward_incoming(
                    &state,
                    &principal.device_id,
                    &session_id,
                    incoming,
                ).await {
                    break;
                }
            }
        }
    }
    state.ssh.remove(&session_id);
    let _ = state
        .nodes
        .send(
            &principal.device_id,
            &ServerToNode::SshSignal {
                session_id,
                signal: "KILL".into(),
            },
        )
        .await;
    let _ = socket.close().await;
}

async fn receive_start(
    socket: &mut WebSocket,
) -> Option<(String, Option<rc_protocol::TerminalSpec>)> {
    let message = tokio::time::timeout(Duration::from_secs(10), socket.recv())
        .await
        .ok()??
        .ok()?;
    let axum::extract::ws::Message::Text(text) = message else {
        return None;
    };
    parse_start(&text)
}

fn insert_process(
    state: &AppState,
    principal: &SshPrincipal,
    process_id: &str,
    terminal: bool,
) -> rusqlite::Result<()> {
    state.db.with_connection(|db| {
        db.execute(
            "INSERT INTO processes(id,device_id,origin,status,terminal,created_by,created_at) \
             VALUES(?,?,'ssh','starting',?,?,?)",
            rusqlite::params![
                process_id,
                principal.device_id,
                i64::from(terminal),
                principal.user_id,
                now_ms(),
            ],
        )?;
        Ok(())
    })
}

fn mark_process_lost(state: &AppState, process_id: &str, reason: &str) {
    if let Err(error) = state.db.with_connection(|db| {
        db.execute(
            "UPDATE processes SET status='lost',error=?,completed_at=? WHERE id=?",
            rusqlite::params![reason, now_ms(), process_id],
        )?;
        Ok(())
    }) {
        tracing::error!(%error, %process_id, "failed to mark SSH process lost");
    }
}

fn touch_key(state: &AppState, key_id: &str) {
    if let Err(error) = state.db.with_connection(|db| {
        db.execute(
            "UPDATE ssh_keys SET last_used=? WHERE id=?",
            rusqlite::params![now_ms(), key_id],
        )?;
        Ok(())
    }) {
        tracing::warn!(%error, %key_id, "failed to update SSH key usage");
    }
}

fn shell_command(original: &str) -> String {
    let value = original.trim();
    if value.is_empty() {
        return "exec \"${SHELL:-sh}\" -l".into();
    }
    if value == "internal-sftp" || value.starts_with("internal-sftp ") {
        return SFTP_COMMAND.into();
    }
    original.to_owned()
}

#[cfg(test)]
mod tests {
    use super::{SFTP_COMMAND, shell_command};

    #[test]
    fn maps_interactive_and_sftp_commands() {
        assert_eq!(shell_command(""), "exec \"${SHELL:-sh}\" -l");
        assert_eq!(shell_command("internal-sftp -d /tmp"), SFTP_COMMAND);
    }

    #[test]
    fn preserves_regular_ssh_commands() {
        assert_eq!(
            shell_command("printf 'hello world'"),
            "printf 'hello world'"
        );
    }
}
