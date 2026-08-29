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
    pub scrollback_bytes: u32,
    pub stdin_chunk_bytes: u32,
    pub authorization_timeout_ms: u32,
    pub terminate_grace_ms: u32,
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
