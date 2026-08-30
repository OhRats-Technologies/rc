use rc_protocol::TerminalSpec;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ProcessChannel {
    Control,
    Ssh,
    Mcp,
    Schedule,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ProcessLifetime {
    Attached,
    Managed,
    Scheduled,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(tag = "kind", rename_all = "camelCase")]
pub enum ProcessExecutionMode {
    Argv { program: String, args: Vec<String> },
    RcShell { script: String },
    SystemShell { command: String },
    SystemLoginShell,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
pub enum ProcessEnvironmentBase {
    Inherit,
    Clean,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
pub struct ProcessEnvironmentChange {
    pub name: String,
    pub value: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
pub struct ProcessEnvironment {
    pub base: ProcessEnvironmentBase,
    pub changes: Vec<ProcessEnvironmentChange>,
}

impl Default for ProcessEnvironment {
    fn default() -> Self {
        Self {
            base: ProcessEnvironmentBase::Inherit,
            changes: Vec::new(),
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ProcessSignal {
    Interrupt,
    Terminate,
    Kill,
}

impl ProcessSignal {
    pub fn parse(value: &str) -> Result<Self, String> {
        match value.trim().to_ascii_uppercase().as_str() {
            "INT" => Ok(Self::Interrupt),
            "TERM" => Ok(Self::Terminate),
            "KILL" => Ok(Self::Kill),
            _ => Err("unsupported process signal".into()),
        }
    }

    pub fn legacy_name(self) -> &'static str {
        match self {
            Self::Interrupt => "INT",
            Self::Terminate => "TERM",
            Self::Kill => "KILL",
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ProcessAction {
    Observe,
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
    pub execution_id: String,
    pub mode: ProcessExecutionMode,
    pub cwd: Option<String>,
    pub environment: ProcessEnvironment,
    pub terminal: Option<TerminalSpec>,
    pub channel: ProcessChannel,
    pub lifetime: ProcessLifetime,
    pub principal: ProcessPrincipal,
    pub max_runtime_ms: Option<u64>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProcessStartPlan {
    pub mode: ProcessExecutionMode,
    pub cwd: Option<String>,
    pub environment: ProcessEnvironment,
    pub terminal: Option<TerminalSpec>,
    pub scrollback_bytes: u32,
    pub stdin_chunk_bytes: u32,
    pub authorization_timeout_ms: u32,
    pub terminate_grace_ms: u32,
    pub reattach_grace_ms: u32,
    pub max_runtime_ms: Option<u64>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProcessAccessRequest {
    pub execution_id: String,
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
    pub signal: ProcessSignal,
}

pub trait ProcessPolicy: Send + Sync {
    fn authorize_start(&self, request: ProcessStartRequest) -> Result<ProcessStartPlan, String>;
    fn authorize_access(&self, request: ProcessAccessRequest) -> Result<(), String>;
    fn normalize_resize(
        &self,
        request: ProcessResizeRequest,
    ) -> Result<ProcessTerminalSize, String>;
    fn authorize_signal(&self, request: ProcessSignalRequest) -> Result<ProcessSignal, String>;
}
