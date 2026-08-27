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

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(tag = "type")]
pub enum NodeToServer {
    #[serde(rename = "hello")]
    Hello {
        #[serde(rename = "agentVersion")]
        agent_version: String,
        hostname: String,
        platform: String,
        arch: String,
        capabilities: Vec<String>,
        #[serde(
            default,
            rename = "transportPublicKey",
            skip_serializing_if = "String::is_empty"
        )]
        transport_public_key: String,
        #[serde(default, rename = "lockHash", skip_serializing_if = "String::is_empty")]
        lock_hash: String,
        #[serde(
            default,
            rename = "lockGeneration",
            skip_serializing_if = "is_zero_u64"
        )]
        lock_generation: u64,
    },
    #[serde(rename = "heartbeat")]
    Heartbeat,
    #[serde(rename = "process.sync")]
    ProcessSync { ids: Vec<String> },
    #[serde(rename = "process.started")]
    ProcessStarted { id: String },
    #[serde(rename = "process.start.request")]
    ProcessStartRequest {
        id: String,
        #[serde(rename = "userId")]
        user_id: String,
    },
    #[serde(rename = "process.stdout")]
    ProcessStdout { id: String, data: String },
    #[serde(rename = "process.stderr")]
    ProcessStderr { id: String, data: String },
    #[serde(rename = "process.exit")]
    ProcessExit {
        id: String,
        #[serde(default, rename = "exitCode", skip_serializing_if = "Option::is_none")]
        exit_code: Option<i32>,
        #[serde(default, skip_serializing_if = "String::is_empty")]
        signal: String,
    },
    #[serde(rename = "node.update.ready")]
    NodeUpdateReady {
        #[serde(
            default,
            rename = "agentVersion",
            skip_serializing_if = "String::is_empty"
        )]
        agent_version: String,
    },
    #[serde(rename = "node.update.error")]
    NodeUpdateError {
        #[serde(default)]
        output: String,
    },
    #[serde(rename = "control.challenge")]
    ControlChallenge {
        #[serde(rename = "requestId")]
        request_id: String,
        challenge: String,
    },
    #[serde(rename = "control.ready")]
    ControlReady {
        #[serde(rename = "requestId")]
        request_id: String,
        #[serde(rename = "sessionId")]
        session_id: String,
        #[serde(rename = "transportPublicKey")]
        transport_public_key: String,
        #[serde(rename = "ephemeralPublicKey")]
        ephemeral_public_key: String,
        signature: String,
    },
    #[serde(rename = "control.webrtc.ready")]
    ControlWebRtcReady {
        #[serde(rename = "requestId")]
        request_id: String,
        #[serde(rename = "sessionId")]
        session_id: String,
        sdp: String,
    },
    #[serde(rename = "control.error")]
    ControlError {
        #[serde(
            default,
            rename = "requestId",
            skip_serializing_if = "String::is_empty"
        )]
        request_id: String,
        output: String,
    },
    #[serde(rename = "control.closed")]
    ControlClosed {
        #[serde(rename = "sessionId")]
        session_id: String,
    },
    #[serde(rename = "lock.state")]
    LockState {
        #[serde(rename = "lockHash")]
        lock_hash: String,
        #[serde(rename = "lockGeneration")]
        lock_generation: u64,
    },
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(tag = "type")]
pub enum ServerToNode {
    #[serde(rename = "lock.bootstrap")]
    LockBootstrap { snapshot: String },
    #[serde(rename = "lock.sync")]
    LockSync {
        snapshot: String,
        #[serde(rename = "previousHash")]
        previous_hash: String,
        #[serde(rename = "previousGeneration")]
        previous_generation: u64,
        grant: String,
        #[serde(rename = "credentialId")]
        credential_id: String,
        assertion: String,
        signature: String,
    },
    #[serde(rename = "process.permit")]
    ProcessPermit {
        id: String,
        #[serde(rename = "userId")]
        user_id: String,
    },
    #[serde(rename = "control.challenge")]
    ControlChallenge {
        #[serde(rename = "requestId")]
        request_id: String,
    },
    #[serde(rename = "control.open")]
    ControlOpen {
        #[serde(rename = "requestId")]
        request_id: String,
        challenge: String,
        #[serde(rename = "clientId")]
        client_id: String,
        grant: String,
        #[serde(rename = "credentialId")]
        credential_id: String,
        assertion: String,
        #[serde(rename = "publicKey")]
        public_key: String,
        signature: String,
    },
    #[serde(rename = "control.webrtc")]
    ControlWebRtc {
        #[serde(rename = "requestId")]
        request_id: String,
        #[serde(rename = "sessionId")]
        session_id: String,
        sdp: String,
        #[serde(rename = "iceServers")]
        ice_servers: Vec<IceServer>,
    },
    #[serde(rename = "control.close")]
    ControlClose {
        #[serde(rename = "sessionId")]
        session_id: String,
    },
    #[serde(rename = "mcp.process.start")]
    McpProcessStart {
        id: String,
        #[serde(rename = "userId")]
        user_id: String,
        command: String,
        #[serde(default)]
        cwd: String,
        #[serde(rename = "mcpGrant")]
        mcp_grant: String,
        #[serde(rename = "mcpSignature")]
        mcp_signature: String,
        grant: String,
        #[serde(rename = "credentialId")]
        credential_id: String,
        assertion: String,
    },
    #[serde(rename = "ssh.process.start")]
    SshProcessStart {
        id: String,
        #[serde(rename = "sessionId")]
        session_id: String,
        #[serde(rename = "userId")]
        user_id: String,
        command: String,
        #[serde(default)]
        cwd: String,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        terminal: Option<TerminalSpec>,
        grant: String,
        #[serde(rename = "credentialId")]
        credential_id: String,
        assertion: String,
    },
    #[serde(rename = "ssh.process.stdin")]
    SshProcessStdin {
        id: String,
        #[serde(rename = "sessionId")]
        session_id: String,
        data: String,
    },
    #[serde(rename = "ssh.process.stdin.close")]
    SshProcessStdinClose {
        id: String,
        #[serde(rename = "sessionId")]
        session_id: String,
    },
    #[serde(rename = "ssh.process.resize")]
    SshProcessResize {
        id: String,
        #[serde(rename = "sessionId")]
        session_id: String,
        cols: u16,
        rows: u16,
    },
    #[serde(rename = "ssh.process.signal")]
    SshProcessSignal {
        id: String,
        #[serde(rename = "sessionId")]
        session_id: String,
        signal: String,
    },
}

fn is_zero_u64(value: &u64) -> bool {
    *value == 0
}
