use crate::{
    ProcessChannel, ProcessEnvironment, ProcessExecutionMode, ProcessLifetime, ProcessPrincipal,
};
use rc_protocol::TerminalSpec;

#[derive(Debug, Clone)]
pub struct ProcessSpec {
    pub id: String,
    pub mode: ProcessExecutionMode,
    pub environment: ProcessEnvironment,
    pub cwd: String,
    pub terminal: Option<TerminalSpec>,
    pub session_id: String,
    pub user_id: String,
    pub authorization_id: String,
    pub secure: bool,
    pub relay_id: String,
    pub scrollback_bytes: u32,
    pub stdin_chunk_bytes: u32,
    pub terminate_grace_ms: u32,
    pub reattach_grace_ms: u32,
    pub lifetime: ProcessLifetime,
    pub channel: ProcessChannel,
    pub principal: ProcessPrincipal,
    pub max_runtime_ms: Option<u64>,
}

impl ProcessSpec {
    pub fn command(id: &str, command: &str) -> Self {
        Self {
            id: id.into(),
            mode: ProcessExecutionMode::SystemShell {
                command: command.into(),
            },
            environment: ProcessEnvironment::default(),
            cwd: String::new(),
            terminal: None,
            session_id: String::new(),
            user_id: String::new(),
            authorization_id: String::new(),
            secure: false,
            relay_id: String::new(),
            scrollback_bytes: 0,
            stdin_chunk_bytes: 1 << 20,
            terminate_grace_ms: 350,
            reattach_grace_ms: 60_000,
            lifetime: ProcessLifetime::Managed,
            channel: ProcessChannel::Control,
            principal: ProcessPrincipal {
                user_id: String::new(),
                role: "owner".into(),
                can_execute: true,
                can_manage_devices: true,
            },
            max_runtime_ms: None,
        }
    }
}
