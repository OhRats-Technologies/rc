use rc_protocol::TerminalSpec;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ProcessChannel {
    Control,
    Ssh,
    Mcp,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ProcessAction {
    Attach,
    Input,
    CloseInput,
    Resize,
    Signal,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProcessPrincipal {
    pub user_id: String,
    pub role: String,
    pub can_execute: bool,
    pub can_manage_devices: bool,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProcessStartRequest {
    pub process_id: String,
    pub command: String,
    pub cwd: String,
    pub terminal: Option<TerminalSpec>,
    pub channel: ProcessChannel,
    pub principal: ProcessPrincipal,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProcessStartPlan {
    pub command: String,
    pub cwd: String,
    pub terminal: Option<TerminalSpec>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProcessAccessRequest {
    pub process_id: String,
    pub owner_user_id: String,
    pub action: ProcessAction,
    pub principal: ProcessPrincipal,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProcessResizeRequest {
    pub access: ProcessAccessRequest,
    pub cols: u16,
    pub rows: u16,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct ProcessTerminalSize {
    pub cols: u16,
    pub rows: u16,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProcessSignalRequest {
    pub access: ProcessAccessRequest,
    pub signal: String,
}

pub trait ProcessPolicy: Send + Sync {
    fn authorize_start(&self, request: ProcessStartRequest) -> Result<ProcessStartPlan, String>;
    fn authorize_access(&self, request: ProcessAccessRequest) -> Result<(), String>;
    fn normalize_resize(
        &self,
        request: ProcessResizeRequest,
    ) -> Result<ProcessTerminalSize, String>;
    fn normalize_signal(&self, request: ProcessSignalRequest) -> Result<String, String>;
}

#[derive(Debug, Default)]
pub struct NativeProcessPolicy;

impl ProcessPolicy for NativeProcessPolicy {
    fn authorize_start(&self, request: ProcessStartRequest) -> Result<ProcessStartPlan, String> {
        if !request.principal.can_execute {
            return Err("execute scope required".into());
        }
        validate_id(&request.process_id)?;
        let command = request.command.trim().to_owned();
        if command.is_empty() || command.len() > 131_072 {
            return Err("invalid process command".into());
        }
        if request.cwd.len() > 4_096 || request.cwd.contains('\0') {
            return Err("invalid process working directory".into());
        }
        let terminal = request.terminal.map(normalize_terminal).transpose()?;
        Ok(ProcessStartPlan {
            command,
            cwd: request.cwd,
            terminal,
        })
    }

    fn authorize_access(&self, request: ProcessAccessRequest) -> Result<(), String> {
        if !request.principal.can_execute {
            return Err("execute scope required".into());
        }
        validate_id(&request.process_id)?;
        if request.principal.role != "owner" && request.owner_user_id != request.principal.user_id {
            return Err("process access denied".into());
        }
        Ok(())
    }

    fn normalize_resize(
        &self,
        request: ProcessResizeRequest,
    ) -> Result<ProcessTerminalSize, String> {
        self.authorize_access(request.access)?;
        if !(2..=500).contains(&request.cols) || !(2..=500).contains(&request.rows) {
            return Err("invalid terminal size".into());
        }
        Ok(ProcessTerminalSize {
            cols: request.cols,
            rows: request.rows,
        })
    }

    fn normalize_signal(&self, request: ProcessSignalRequest) -> Result<String, String> {
        self.authorize_access(request.access)?;
        let signal = request.signal.trim().to_ascii_uppercase();
        match signal.as_str() {
            "INT" | "TERM" | "KILL" => Ok(signal),
            _ => Err("unsupported process signal".into()),
        }
    }
}

fn validate_id(value: &str) -> Result<(), String> {
    if !value.is_empty()
        && value.len() <= 128
        && value
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'_' | b'.'))
    {
        Ok(())
    } else {
        Err("invalid process id".into())
    }
}

fn normalize_terminal(mut value: TerminalSpec) -> Result<TerminalSpec, String> {
    if !(2..=500).contains(&value.cols)
        || !(2..=500).contains(&value.rows)
        || value.term.len() > 128
        || value.term.contains('\0')
    {
        return Err("invalid terminal specification".into());
    }
    value.term = if value.term.trim().is_empty() {
        "xterm-256color".into()
    } else {
        value.term.trim().to_owned()
    };
    Ok(value)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn operator_is_limited_to_owned_processes() {
        let policy = NativeProcessPolicy;
        let request = ProcessAccessRequest {
            process_id: "process-1".into(),
            owner_user_id: "other".into(),
            action: ProcessAction::Signal,
            principal: ProcessPrincipal {
                user_id: "user-1".into(),
                role: "operator".into(),
                can_execute: true,
                can_manage_devices: false,
            },
        };
        assert_eq!(
            policy.authorize_access(request),
            Err("process access denied".into())
        );
    }
}
