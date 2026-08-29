use crate::TerminalSpec;
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct IceServer {
    pub urls: Vec<String>,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub username: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub credential: String,
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum ControlIceMode {
    Host,
    #[default]
    Stun,
    Relay,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ControlIceAttempt {
    pub mode: ControlIceMode,
    pub gather_timeout_ms: u32,
    pub connect_timeout_ms: u32,
    pub retry_delay_ms: u32,
}

pub fn control_attempts_payload(attempts: &[ControlIceAttempt]) -> String {
    attempts
        .iter()
        .map(|attempt| {
            format!(
                "{}:{}:{}:{}",
                match attempt.mode {
                    ControlIceMode::Host => "host",
                    ControlIceMode::Stun => "stun",
                    ControlIceMode::Relay => "relay",
                },
                attempt.gather_timeout_ms,
                attempt.connect_timeout_ms,
                attempt.retry_delay_ms
            )
        })
        .collect::<Vec<_>>()
        .join(",")
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct NodeHello {
    pub version: String,
    pub hostname: String,
    pub platform: String,
    pub arch: String,
    #[serde(default)]
    pub capabilities: Vec<String>,
    pub transport_public_key: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub lock_hash: String,
    #[serde(default)]
    pub lock_generation: u64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum NodeToServer {
    Hello {
        hello: NodeHello,
    },
    ProcessSync {
        ids: Vec<String>,
    },
    ProcessStartRequest {
        id: String,
        user_id: String,
    },
    ProcessStarted {
        id: String,
    },
    ProcessExit {
        id: String,
        exit_code: i32,
        #[serde(default)]
        signal: String,
    },
    ControlChallenge {
        request_id: String,
        challenge: String,
    },
    ControlReady {
        request_id: String,
        session_id: String,
        transport_public_key: String,
        ephemeral_public_key: String,
        signature: String,
        attempts: Vec<ControlIceAttempt>,
    },
    ControlWebrtcAnswer {
        request_id: String,
        session_id: String,
        sdp: String,
    },
    ControlError {
        #[serde(default)]
        request_id: String,
        error: String,
    },
    ControlClosed {
        session_id: String,
    },
    LockState {
        hash: String,
        generation: u64,
    },
    SshStdout {
        session_id: String,
        data: String,
    },
    SshStderr {
        session_id: String,
        data: String,
    },
    SshExit {
        session_id: String,
        exit_code: i32,
        #[serde(default)]
        signal: String,
    },
    McpStdout {
        process_id: String,
        data: String,
    },
    McpStderr {
        process_id: String,
        data: String,
    },
    McpExit {
        process_id: String,
        exit_code: i32,
        #[serde(default)]
        signal: String,
    },
    UpdateResult {
        ok: bool,
        #[serde(default)]
        version: String,
        #[serde(default)]
        error: String,
    },
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum ServerToNode {
    LockBootstrap {
        snapshot: String,
    },
    LockSync {
        snapshot: String,
        previous_hash: String,
        previous_generation: u64,
        grant: String,
        credential_id: String,
        assertion: String,
        signature: String,
    },
    ProcessPermit {
        id: String,
        user_id: String,
    },
    ControlChallenge {
        request_id: String,
    },
    ControlOpen {
        request_id: String,
        challenge: String,
        user_id: String,
        client_id: String,
        grant: String,
        credential_id: String,
        assertion: String,
        public_key: String,
        signature: String,
        ice_servers: Vec<IceServer>,
    },
    ControlWebrtcOffer {
        request_id: String,
        session_id: String,
        sdp: String,
        #[serde(default)]
        mode: ControlIceMode,
        ice_servers: Vec<IceServer>,
    },
    ControlClose {
        session_id: String,
    },
    SshStart {
        process_id: String,
        session_id: String,
        user_id: String,
        command: String,
        #[serde(default)]
        cwd: String,
        terminal: Option<TerminalSpec>,
        grant: String,
        credential_id: String,
        assertion: String,
    },
    SshStdin {
        session_id: String,
        data: String,
    },
    SshStdinClose {
        session_id: String,
    },
    SshResize {
        session_id: String,
        cols: u16,
        rows: u16,
    },
    SshSignal {
        session_id: String,
        signal: String,
    },
    McpStart {
        process_id: String,
        user_id: String,
        command: String,
        #[serde(default)]
        cwd: String,
        mcp_grant: String,
        mcp_signature: String,
        control_grant: String,
        credential_id: String,
        control_assertion: String,
    },
    McpStdin {
        process_id: String,
        data: String,
    },
    McpStdinClose {
        process_id: String,
    },
    McpSignal {
        process_id: String,
        signal: String,
    },
    Update,
}
