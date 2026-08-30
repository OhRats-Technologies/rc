use crate::{
    ControlIceAttempt, ControlIceMode, EnvironmentSpec, ExecutionMode, IceServer,
    McpExecutionChunk, NodeHello, TerminalSpec,
};
use serde::{Deserialize, Serialize};

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
    McpExit {
        process_id: String,
        exit_code: i32,
        #[serde(default)]
        signal: String,
    },
    McpExecutionStatusResult {
        request_id: String,
        process_id: String,
        status: String,
        chunks: Vec<McpExecutionChunk>,
        next_cursor: u64,
        truncated_before_cursor: u64,
        output_pending: bool,
        exit_code: Option<i32>,
        #[serde(default)]
        signal: String,
        #[serde(default)]
        error: String,
    },
    McpExecutionOperationResult {
        request_id: String,
        process_id: String,
        accepted: bool,
        #[serde(default)]
        error: String,
    },
    McpImageChunk {
        request_id: String,
        data: String,
    },
    McpImageResult {
        request_id: String,
        #[serde(default)]
        mime_type: String,
        size_bytes: u64,
        #[serde(default)]
        error: String,
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
        mode: ExecutionMode,
        #[serde(default)]
        cwd: String,
        #[serde(default)]
        environment: EnvironmentSpec,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        max_runtime_seconds: Option<u64>,
        mcp_grant: String,
        mcp_signature: String,
        control_grant: String,
        credential_id: String,
        control_assertion: String,
    },
    McpExecutionInput {
        request_id: String,
        process_id: String,
        user_id: String,
        data: String,
        eof: bool,
        mcp_grant: String,
        mcp_signature: String,
        control_grant: String,
        credential_id: String,
        control_assertion: String,
    },
    McpExecutionSignal {
        request_id: String,
        process_id: String,
        user_id: String,
        signal: String,
        mcp_grant: String,
        mcp_signature: String,
        control_grant: String,
        credential_id: String,
        control_assertion: String,
    },
    McpExecutionStatus {
        request_id: String,
        process_id: String,
        user_id: String,
        cursor: u64,
        wait_seconds: u64,
        mcp_grant: String,
        mcp_signature: String,
        control_grant: String,
        credential_id: String,
        control_assertion: String,
    },
    McpImageView {
        request_id: String,
        user_id: String,
        path: String,
        mcp_grant: String,
        mcp_signature: String,
        control_grant: String,
        credential_id: String,
        control_assertion: String,
    },
    Update,
}
