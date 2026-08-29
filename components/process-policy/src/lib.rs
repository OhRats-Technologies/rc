wit_bindgen::generate!({
    path: "../../wit",
    world: "process-policy",
    generate_all,
});

use exports::ohrats::rc_process::policy::Guest as PolicyGuest;
use ohrats::{
    rc_plugin::types::Service,
    rc_process::types::{
        AccessRequest, ResizeRequest, SignalRequest, StartPlan, StartRequest, Terminal,
        TerminalSize,
    },
};

const MAX_COMMAND: usize = 131_072;
const MAX_CWD: usize = 4_096;
const MAX_TERM: usize = 128;

struct ProcessPolicy;

impl Guest for ProcessPolicy {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:process-policy".into(),
            version: if cfg!(feature = "fixture") {
                "0.2.1"
            } else {
                "0.2.0"
            }
            .into(),
            provides: vec![Service {
                name: "ohrats:rc-process/policy".into(),
                version: "0.2.0".into(),
                priority: 100,
                keys: Vec::new(),
            }],
            requires: Vec::new(),
            commands: Vec::new(),
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }

    fn deactivate() {}

    fn invoke(command: String, _args: Vec<String>) -> Result<u32, String> {
        Err(format!("unsupported command {command:?}"))
    }
}

impl PolicyGuest for ProcessPolicy {
    fn authorize_start(request: StartRequest) -> Result<StartPlan, String> {
        if !request.principal.can_execute {
            return Err("execute scope required".into());
        }
        validate_token(&request.process_id, 128, "process id")?;
        let command = request.command.trim().to_owned();
        if command.is_empty() || command.len() > MAX_COMMAND {
            return Err("invalid process command".into());
        }
        if request.cwd.len() > MAX_CWD || request.cwd.contains('\0') {
            return Err("invalid process working directory".into());
        }
        let terminal = request.terminal.map(normalize_terminal).transpose()?;
        Ok(StartPlan {
            command,
            cwd: request.cwd,
            terminal,
            scrollback_bytes: 4 << 20,
            stdin_chunk_bytes: if cfg!(feature = "fixture") {
                64
            } else {
                1 << 20
            },
            authorization_timeout_ms: 15_000,
            terminate_grace_ms: 350,
        })
    }

    fn authorize_access(request: AccessRequest) -> Result<(), String> {
        if !request.principal.can_execute {
            return Err("execute scope required".into());
        }
        validate_token(&request.process_id, 128, "process id")?;
        if request.principal.role != "owner" && request.owner_user_id != request.principal.user_id {
            return Err("process access denied".into());
        }
        Ok(())
    }

    fn normalize_resize(request: ResizeRequest) -> Result<TerminalSize, String> {
        Self::authorize_access(request.access)?;
        if !(2..=500).contains(&request.cols) || !(2..=500).contains(&request.rows) {
            return Err("invalid terminal size".into());
        }
        Ok(TerminalSize {
            cols: request.cols,
            rows: request.rows,
        })
    }

    fn normalize_signal(request: SignalRequest) -> Result<String, String> {
        Self::authorize_access(request.access)?;
        let signal = request.signal.trim().to_ascii_uppercase();
        match signal.as_str() {
            "INT" | "TERM" | "KILL" => Ok(signal),
            _ => Err("unsupported process signal".into()),
        }
    }
}

fn normalize_terminal(value: Terminal) -> Result<Terminal, String> {
    if !(2..=500).contains(&value.cols)
        || !(2..=500).contains(&value.rows)
        || value.term.len() > MAX_TERM
        || value.term.contains('\0')
    {
        return Err("invalid terminal specification".into());
    }
    Ok(Terminal {
        cols: value.cols,
        rows: value.rows,
        term: if value.term.trim().is_empty() {
            "xterm-256color".into()
        } else {
            value.term.trim().to_owned()
        },
    })
}

fn validate_token(value: &str, maximum: usize, label: &str) -> Result<(), String> {
    if !value.is_empty()
        && value.len() <= maximum
        && value
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'_' | b'.'))
    {
        Ok(())
    } else {
        Err(format!("invalid {label}"))
    }
}

export!(ProcessPolicy);

#[cfg(test)]
mod tests {
    use super::*;
    use ohrats::rc_process::types::{Action, Channel, Principal};

    fn principal(role: &str, execute: bool) -> Principal {
        Principal {
            user_id: "user-1".into(),
            role: role.into(),
            can_execute: execute,
            can_manage_devices: role == "owner",
        }
    }

    #[test]
    fn owner_can_access_another_users_process() {
        let request = AccessRequest {
            process_id: "process-1".into(),
            owner_user_id: "user-2".into(),
            action: Action::Signal,
            principal: principal("owner", true),
        };
        assert!(ProcessPolicy::authorize_access(request).is_ok());
    }

    #[test]
    fn operator_cannot_access_another_users_process() {
        let request = AccessRequest {
            process_id: "process-1".into(),
            owner_user_id: "user-2".into(),
            action: Action::Attach,
            principal: principal("operator", true),
        };
        assert_eq!(
            ProcessPolicy::authorize_access(request),
            Err("process access denied".into())
        );
    }

    #[test]
    fn start_normalizes_terminal_name() {
        let request = StartRequest {
            process_id: "process-1".into(),
            command: " printf ok ".into(),
            cwd: String::new(),
            terminal: Some(Terminal {
                cols: 80,
                rows: 24,
                term: " ".into(),
            }),
            channel: Channel::Control,
            principal: principal("operator", true),
        };
        let plan = ProcessPolicy::authorize_start(request).unwrap();
        assert_eq!(plan.command, "printf ok");
        assert_eq!(plan.terminal.unwrap().term, "xterm-256color");
    }
}
