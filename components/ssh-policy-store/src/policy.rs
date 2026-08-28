use crate::{
    key,
    ohrats::rc_ssh::types::{CommandKind, SessionPolicy, SessionRequest, WorkspaceRole},
};

const SFTP: &str = r#"if command -v sftp-server >/dev/null 2>&1; then exec "$(command -v sftp-server)"; elif [ -x /usr/lib/openssh/sftp-server ]; then exec /usr/lib/openssh/sftp-server; elif [ -x /usr/lib/ssh/sftp-server ]; then exec /usr/lib/ssh/sftp-server; elif [ -x /usr/libexec/sftp-server ]; then exec /usr/libexec/sftp-server; else echo 'sftp-server not installed' >&2; exit 127; fi"#;
const OPTIONS: &str = "no-agent-forwarding,no-port-forwarding,no-X11-forwarding,no-user-rc,no-pty";

pub fn authorize(request: SessionRequest) -> Result<SessionPolicy, String> {
    if request.device_id.trim().is_empty()
        || request.device_id.len() > 128
        || request.device_id.bytes().any(|b| b.is_ascii_whitespace())
    {
        return Err("invalid immutable device id".into());
    }
    if matches!(request.workspace_role, WorkspaceRole::Viewer) {
        return Err("workspace role may not execute".into());
    }
    if request.agent_forwarding
        || request.port_forwarding
        || request.x11_forwarding
        || request.tunnel
    {
        return Err("SSH forwarding and tunnels are forbidden".into());
    }
    if request.control_client_expires_at_ms != 0
        && request.control_client_expires_at_ms <= request.requested_at_ms
    {
        return Err("control client authorization expired".into());
    }
    let stored = key::get(&request.key_id)?.ok_or("SSH key not found")?;
    if stored.user_id != request.user_id || stored.control_client_id != request.control_client_id {
        return Err("SSH key principal mismatch".into());
    }
    let original = request.original_command;
    if original.len() > 16 * 1024 {
        return Err("SSH command exceeds session limit".into());
    }
    let trimmed = original.trim();
    let (kind, command, terminal) = if trimmed.is_empty() {
        (
            CommandKind::Shell,
            "exec \"${SHELL:-sh}\" -l".into(),
            request.terminal,
        )
    } else if trimmed == "internal-sftp" || trimmed.starts_with("internal-sftp ") {
        (CommandKind::Sftp, SFTP.into(), false)
    } else if trimmed.starts_with("scp ") {
        (CommandKind::Scp, original, false)
    } else if trimmed.starts_with("rsync --server ") {
        (CommandKind::Rsync, original, false)
    } else {
        (CommandKind::Exec, original, request.terminal)
    };
    Ok(SessionPolicy {
        device_id: request.device_id,
        command_kind: kind,
        command,
        terminal,
        max_duration_seconds: 86_400,
        max_input_bytes: 64 * 1024 * 1024,
        max_output_bytes: 256 * 1024 * 1024,
    })
}

pub fn authorized_key_line(algorithm: &str, data: &str) -> Result<Option<String>, String> {
    let Some(key) = key::find(algorithm, data)? else {
        return Ok(None);
    };
    let command = format!("/usr/local/bin/rc-ssh-bridge {}", key.id);
    Ok(Some(format!(
        "{OPTIONS},command=\"{command}\" {} {}",
        key.algorithm, key.key_data
    )))
}
