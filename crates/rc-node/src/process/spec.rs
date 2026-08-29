use rc_protocol::TerminalSpec;

#[derive(Debug, Clone)]
pub struct ProcessSpec {
    pub id: String,
    pub command: String,
    pub cwd: String,
    pub terminal: Option<TerminalSpec>,
    pub session_id: String,
    pub user_id: String,
    pub secure: bool,
    pub relay_id: String,
    pub scrollback_bytes: u32,
    pub stdin_chunk_bytes: u32,
    pub terminate_grace_ms: u32,
}

impl ProcessSpec {
    pub fn command(id: &str, command: &str) -> Self {
        Self {
            id: id.into(),
            command: command.into(),
            cwd: String::new(),
            terminal: None,
            session_id: String::new(),
            user_id: String::new(),
            secure: false,
            relay_id: String::new(),
            scrollback_bytes: 0,
            stdin_chunk_bytes: 1 << 20,
            terminate_grace_ms: 350,
        }
    }
}
