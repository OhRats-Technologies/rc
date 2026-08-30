use crate::component::ohrats::{
    rc_process::types::{Environment, EnvironmentBase, EnvironmentChange, ExecutionMode},
    rc_scheduler::types::{Definition, MisfirePolicy, OverlapPolicy},
};
use serde::{Deserialize, Serialize};

#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredDefinition {
    id: String,
    name: Option<String>,
    cron: String,
    timezone: String,
    mode: StoredMode,
    cwd: Option<String>,
    environment: StoredEnvironment,
    enabled: bool,
    overlap: String,
    misfire: String,
    max_runtime_ms: Option<u64>,
    permit_hash: String,
    created_by: String,
    created_at_ms: u64,
    expires_at_ms: Option<u64>,
}

#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredCursor {
    pub checked_at_ms: u64,
    pub last_occurrence_id: Option<String>,
}

#[derive(Serialize, Deserialize)]
#[serde(tag = "kind", rename_all = "camelCase")]
enum StoredMode {
    Argv { program: String, args: Vec<String> },
    RcShell { script: String },
    SystemShell { command: String },
    SystemLoginShell,
}

#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct StoredEnvironment {
    clean: bool,
    changes: Vec<(String, Option<String>)>,
}

impl From<Definition> for StoredDefinition {
    fn from(value: Definition) -> Self {
        Self {
            id: value.id,
            name: value.name,
            cron: value.cron,
            timezone: value.timezone,
            mode: value.mode.into(),
            cwd: value.cwd,
            environment: value.environment.into(),
            enabled: value.enabled,
            overlap: "forbid".into(),
            misfire: match value.misfire {
                MisfirePolicy::Skip => "skip",
                MisfirePolicy::RunOnce => "run-once",
            }
            .into(),
            max_runtime_ms: value.max_runtime_ms,
            permit_hash: value.permit_hash,
            created_by: value.created_by,
            created_at_ms: value.created_at_ms,
            expires_at_ms: value.expires_at_ms,
        }
    }
}

impl TryFrom<StoredDefinition> for Definition {
    type Error = String;

    fn try_from(value: StoredDefinition) -> Result<Self, Self::Error> {
        Ok(Self {
            id: value.id,
            name: value.name,
            cron: value.cron,
            timezone: value.timezone,
            mode: value.mode.into(),
            cwd: value.cwd,
            environment: value.environment.into(),
            enabled: value.enabled,
            overlap: match value.overlap.as_str() {
                "forbid" => OverlapPolicy::Forbid,
                _ => return Err("invalid stored overlap policy".into()),
            },
            misfire: match value.misfire.as_str() {
                "skip" => MisfirePolicy::Skip,
                "run-once" => MisfirePolicy::RunOnce,
                _ => return Err("invalid stored misfire policy".into()),
            },
            max_runtime_ms: value.max_runtime_ms,
            permit_hash: value.permit_hash,
            created_by: value.created_by,
            created_at_ms: value.created_at_ms,
            expires_at_ms: value.expires_at_ms,
        })
    }
}

impl From<ExecutionMode> for StoredMode {
    fn from(value: ExecutionMode) -> Self {
        match value {
            ExecutionMode::Argv((program, args)) => Self::Argv { program, args },
            ExecutionMode::RcShell(script) => Self::RcShell { script },
            ExecutionMode::SystemShell(command) => Self::SystemShell { command },
            ExecutionMode::SystemLoginShell => Self::SystemLoginShell,
        }
    }
}

impl From<StoredMode> for ExecutionMode {
    fn from(value: StoredMode) -> Self {
        match value {
            StoredMode::Argv { program, args } => Self::Argv((program, args)),
            StoredMode::RcShell { script } => Self::RcShell(script),
            StoredMode::SystemShell { command } => Self::SystemShell(command),
            StoredMode::SystemLoginShell => Self::SystemLoginShell,
        }
    }
}

impl From<Environment> for StoredEnvironment {
    fn from(value: Environment) -> Self {
        Self {
            clean: matches!(value.base, EnvironmentBase::Clean),
            changes: value
                .changes
                .into_iter()
                .map(|change| (change.name, change.value))
                .collect(),
        }
    }
}

impl From<StoredEnvironment> for Environment {
    fn from(value: StoredEnvironment) -> Self {
        Self {
            base: if value.clean {
                EnvironmentBase::Clean
            } else {
                EnvironmentBase::Inherit
            },
            changes: value
                .changes
                .into_iter()
                .map(|(name, value)| EnvironmentChange { name, value })
                .collect(),
        }
    }
}
