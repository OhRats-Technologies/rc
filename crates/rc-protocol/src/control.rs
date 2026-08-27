use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TerminalSpec {
    #[serde(default, skip_serializing_if = "is_zero_u16")]
    pub cols: u16,
    #[serde(default, skip_serializing_if = "is_zero_u16")]
    pub rows: u16,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub term: String,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(tag = "type")]
pub enum ControlMessage {
    #[serde(rename = "process.start")]
    ProcessStart {
        id: String,
        command: String,
        #[serde(default, skip_serializing_if = "String::is_empty")]
        cwd: String,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        terminal: Option<TerminalSpec>,
    },
    #[serde(rename = "process.attach")]
    ProcessAttach { id: String },
    #[serde(rename = "process.stdin")]
    ProcessStdin { id: String, data: String },
    #[serde(rename = "process.stdin.close")]
    ProcessStdinClose { id: String },
    #[serde(rename = "process.resize")]
    ProcessResize { id: String, cols: u16, rows: u16 },
    #[serde(rename = "process.signal")]
    ProcessSignal { id: String, signal: String },
    #[serde(rename = "process.stdout")]
    ProcessStdout { id: String, data: String },
    #[serde(rename = "process.stderr")]
    ProcessStderr { id: String, data: String },
    #[serde(rename = "process.started")]
    ProcessStarted { id: String },
    #[serde(rename = "process.exit")]
    ProcessExit {
        id: String,
        #[serde(default, rename = "exitCode", skip_serializing_if = "Option::is_none")]
        exit_code: Option<i32>,
        #[serde(default, skip_serializing_if = "String::is_empty")]
        signal: String,
    },
    #[serde(rename = "node.update")]
    NodeUpdate {
        #[serde(
            default,
            rename = "requestId",
            skip_serializing_if = "String::is_empty"
        )]
        request_id: String,
    },
    #[serde(rename = "control.result")]
    Result {
        #[serde(rename = "requestId")]
        request_id: String,
        output: String,
    },
    #[serde(rename = "control.revoked")]
    Revoked,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(tag = "type")]
pub enum ControlTransportMessage {
    #[serde(rename = "control.frame")]
    Frame {
        #[serde(rename = "sessionId")]
        session_id: String,
        sequence: u64,
        ciphertext: String,
    },
}

fn is_zero_u16(value: &u16) -> bool {
    *value == 0
}
