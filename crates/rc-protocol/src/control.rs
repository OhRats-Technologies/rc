use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

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
#[serde(tag = "kind", rename_all = "camelCase")]
pub enum ExecutionMode {
    Argv { program: String, args: Vec<String> },
    RcShell { script: String },
    SystemShell { command: String },
    SystemLoginShell,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum EnvironmentBase {
    #[default]
    Inherit,
    Clean,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct EnvironmentChange {
    pub name: String,
    pub value: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Eq, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct EnvironmentSpec {
    #[serde(default)]
    pub base: EnvironmentBase,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub changes: Vec<EnvironmentChange>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum ScheduleMisfirePolicy {
    Skip,
    RunOnce,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ScheduleDefinition {
    pub id: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    pub cron: String,
    pub timezone: String,
    pub mode: ExecutionMode,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub cwd: Option<String>,
    #[serde(default, skip_serializing_if = "is_default_environment")]
    pub environment: EnvironmentSpec,
    pub enabled: bool,
    pub misfire: ScheduleMisfirePolicy,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub max_runtime_ms: Option<u64>,
    pub permit_hash: String,
    pub created_by: String,
    pub created_at_ms: u64,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub expires_at_ms: Option<u64>,
}

pub fn schedule_spec_hash(schedule: &ScheduleDefinition) -> String {
    #[derive(Serialize)]
    #[serde(rename_all = "camelCase")]
    struct Immutable<'a> {
        id: &'a str,
        cron: &'a str,
        timezone: &'a str,
        mode: &'a ExecutionMode,
        cwd: &'a Option<String>,
        environment: &'a EnvironmentSpec,
        misfire: ScheduleMisfirePolicy,
        max_runtime_ms: &'a Option<u64>,
        created_by: &'a str,
        expires_at_ms: &'a Option<u64>,
    }
    let bytes = serde_json::to_vec(&Immutable {
        id: &schedule.id,
        cron: &schedule.cron,
        timezone: &schedule.timezone,
        mode: &schedule.mode,
        cwd: &schedule.cwd,
        environment: &schedule.environment,
        misfire: schedule.misfire,
        max_runtime_ms: &schedule.max_runtime_ms,
        created_by: &schedule.created_by,
        expires_at_ms: &schedule.expires_at_ms,
    })
    .expect("schedule authority fields serialize");
    format!("{:x}", Sha256::digest(bytes))
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(tag = "type")]
pub enum ControlMessage {
    #[serde(rename = "schedule.list")]
    ScheduleList {
        #[serde(rename = "requestId")]
        request_id: String,
    },
    #[serde(rename = "schedule.upsert")]
    ScheduleUpsert {
        #[serde(rename = "requestId")]
        request_id: String,
        schedule: ScheduleDefinition,
    },
    #[serde(rename = "schedule.remove")]
    ScheduleRemove {
        #[serde(rename = "requestId")]
        request_id: String,
        id: String,
    },
    #[serde(rename = "schedule.setEnabled")]
    ScheduleSetEnabled {
        #[serde(rename = "requestId")]
        request_id: String,
        id: String,
        enabled: bool,
    },
    #[serde(rename = "schedule.result")]
    ScheduleResult {
        #[serde(rename = "requestId")]
        request_id: String,
        #[serde(default, skip_serializing_if = "Vec::is_empty")]
        schedules: Vec<ScheduleDefinition>,
        #[serde(default, skip_serializing_if = "String::is_empty")]
        error: String,
    },
    #[serde(rename = "process.start")]
    ProcessStart {
        id: String,
        mode: ExecutionMode,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        cwd: Option<String>,
        #[serde(default, skip_serializing_if = "is_default_environment")]
        environment: EnvironmentSpec,
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

fn is_default_environment(value: &EnvironmentSpec) -> bool {
    value == &EnvironmentSpec::default()
}

#[cfg(test)]
mod tests;
