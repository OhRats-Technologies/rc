wit_bindgen::generate!({
    path: "../../wit",
    world: "process-policy",
    generate_all,
});

use exports::ohrats::rc_process::policy::Guest as PolicyGuest;
use ohrats::{
    rc_plugin::types::Service,
    rc_process::types::{
        AccessRequest, Channel, EnvironmentBase, ExecutionMode, Lifetime, ResizeRequest, Signal,
        SignalRequest, StartPlan, StartRequest, Terminal, TerminalSize,
    },
};

const MAX_TERM: usize = 128;

struct ProcessPolicy;

impl Guest for ProcessPolicy {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:process-policy".into(),
            version: if cfg!(feature = "fixture") {
                "0.3.1"
            } else {
                "0.3.0"
            }
            .into(),
            provides: vec![Service {
                name: "ohrats:rc-process/policy".into(),
                version: "0.3.0".into(),
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
        validate_token(&request.execution_id, 128, "execution id")?;
        validate_channel(&request)?;
        validate_mode(&request.mode)?;
        if request.cwd.as_deref().is_some_and(|cwd| cwd.contains('\0')) {
            return Err("invalid process working directory".into());
        }
        validate_environment(&request.environment)?;
        let terminal = request.terminal.map(normalize_terminal).transpose()?;
        Ok(StartPlan {
            mode: request.mode,
            cwd: request.cwd,
            environment: request.environment,
            terminal,
            scrollback_bytes: 4 << 20,
            stdin_chunk_bytes: if cfg!(feature = "fixture") {
                64
            } else {
                1 << 20
            },
            authorization_timeout_ms: 15_000,
            terminate_grace_ms: 350,
            reattach_grace_ms: 60_000,
            max_runtime_ms: request.max_runtime_ms,
        })
    }

    fn authorize_access(request: AccessRequest) -> Result<(), String> {
        if !request.principal.can_execute {
            return Err("execute scope required".into());
        }
        validate_token(&request.execution_id, 128, "execution id")?;
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

    fn authorize_signal(request: SignalRequest) -> Result<Signal, String> {
        Self::authorize_access(request.access)?;
        Ok(request.signal)
    }
}

fn validate_channel(request: &StartRequest) -> Result<(), String> {
    let valid = matches!(
        (request.channel, request.lifetime),
        (Channel::Control, Lifetime::Attached | Lifetime::Managed)
            | (Channel::Ssh | Channel::Mcp, Lifetime::Managed)
            | (Channel::Schedule, Lifetime::Scheduled)
    );
    if !valid {
        return Err("execution channel and lifetime are incompatible".into());
    }
    if request.terminal.is_some() && matches!(request.channel, Channel::Mcp | Channel::Schedule) {
        return Err("execution channel does not support a terminal".into());
    }
    Ok(())
}

fn validate_mode(mode: &ExecutionMode) -> Result<(), String> {
    let valid = match mode {
        ExecutionMode::Argv((program, args)) => {
            !program.is_empty()
                && !program.contains('\0')
                && args.iter().all(|arg| !arg.contains('\0'))
        }
        ExecutionMode::RcShell(script) | ExecutionMode::SystemShell(script) => {
            !script.trim().is_empty() && !script.contains('\0')
        }
        ExecutionMode::SystemLoginShell => true,
    };
    valid
        .then_some(())
        .ok_or_else(|| "invalid execution mode".into())
}

fn validate_environment(
    environment: &ohrats::rc_process::types::Environment,
) -> Result<(), String> {
    let mut names = std::collections::BTreeSet::new();
    for change in &environment.changes {
        if change.name.is_empty()
            || change.name.contains(['=', '\0'])
            || change
                .value
                .as_deref()
                .is_some_and(|value| value.contains('\0'))
            || !names.insert(change.name.clone())
        {
            return Err("invalid or conflicting environment change".into());
        }
    }
    match environment.base {
        EnvironmentBase::Inherit | EnvironmentBase::Clean => Ok(()),
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
mod tests;
