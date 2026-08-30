use super::{ComponentScheduler, display};
use crate::descriptor::SelectionMode;
use rc_node::{ProcessEnvironment, ProcessEnvironmentBase, ProcessExecutionMode, ScheduleManager};
use rc_protocol::{
    EnvironmentBase, EnvironmentSpec, ExecutionMode, ScheduleDefinition, ScheduleMisfirePolicy,
};
use wasmtime::component::Val;

const SERVICE: &str = "ohrats:rc-scheduler/management";

impl ScheduleManager for ComponentScheduler {
    fn list(&self) -> Result<Vec<ScheduleDefinition>, String> {
        let values = self.call("list-schedules", &[])?;
        crate::node::values::list(result(values, "schedule list")?, "schedules")?
            .into_iter()
            .map(parse)
            .collect()
    }

    fn upsert(&self, schedule: ScheduleDefinition) -> Result<(), String> {
        crate::node::values::unit_result(
            self.call("upsert-schedule", &[encode(schedule)])?,
            "schedule upsert",
        )?;
        self.notify();
        Ok(())
    }

    fn remove(&self, id: &str) -> Result<bool, String> {
        let removed = boolean(result(
            self.call("remove-schedule", &[Val::String(id.into())])?,
            "remove",
        )?)?;
        if removed {
            self.notify();
        }
        Ok(removed)
    }

    fn set_enabled(&self, id: &str, enabled: bool) -> Result<bool, String> {
        let changed = boolean(result(
            self.call("set-enabled", &[Val::String(id.into()), Val::Bool(enabled)])?,
            "enable",
        )?)?;
        if changed {
            self.notify();
        }
        Ok(changed)
    }
}

impl ComponentScheduler {
    fn call(&self, function: &str, arguments: &[Val]) -> Result<Vec<Val>, String> {
        self.registry
            .call_one(
                SERVICE,
                &self.requirement,
                SelectionMode::Single,
                function,
                arguments,
            )
            .map_err(display)
    }
}

fn parse(value: Val) -> Result<ScheduleDefinition, String> {
    use crate::node::{process::wire, values};
    let fields = values::record(value, "schedule")?;
    Ok(ScheduleDefinition {
        id: values::string_field(&fields, "id")?,
        name: wire::option_string_field(&fields, "name")?,
        cron: values::string_field(&fields, "cron")?,
        timezone: values::string_field(&fields, "timezone")?,
        mode: mode_from_process(wire::mode_field(&fields, "mode")?),
        cwd: wire::option_string_field(&fields, "cwd")?,
        environment: environment_from_process(wire::environment_field(&fields, "environment")?),
        enabled: boolean(values::field(&fields, "enabled")?.clone())?,
        misfire: match values::enum_field(&fields, "misfire")?.as_str() {
            "skip" => ScheduleMisfirePolicy::Skip,
            "run-once" => ScheduleMisfirePolicy::RunOnce,
            _ => return Err("invalid schedule misfire policy".into()),
        },
        max_runtime_ms: wire::option_u64_field(&fields, "max-runtime-ms")?,
        permit_hash: values::string_field(&fields, "permit-hash")?,
        created_by: values::string_field(&fields, "created-by")?,
        created_at_ms: u64_value(values::field(&fields, "created-at-ms")?.clone())?,
        expires_at_ms: wire::option_u64_field(&fields, "expires-at-ms")?,
    })
}

fn encode(value: ScheduleDefinition) -> Val {
    use crate::node::process::wire;
    Val::Record(vec![
        ("id".into(), Val::String(value.id)),
        ("name".into(), wire::option_string(value.name)),
        ("cron".into(), Val::String(value.cron)),
        ("timezone".into(), Val::String(value.timezone)),
        ("mode".into(), wire::mode_value(mode_to_process(value.mode))),
        ("cwd".into(), wire::option_string(value.cwd)),
        (
            "environment".into(),
            wire::environment_value(environment_to_process(value.environment)),
        ),
        ("enabled".into(), Val::Bool(value.enabled)),
        ("overlap".into(), Val::Enum("forbid".into())),
        (
            "misfire".into(),
            Val::Enum(misfire_name(value.misfire).into()),
        ),
        (
            "max-runtime-ms".into(),
            wire::option_u64(value.max_runtime_ms),
        ),
        ("permit-hash".into(), Val::String(value.permit_hash)),
        ("created-by".into(), Val::String(value.created_by)),
        ("created-at-ms".into(), Val::U64(value.created_at_ms)),
        (
            "expires-at-ms".into(),
            wire::option_u64(value.expires_at_ms),
        ),
    ])
}

fn mode_to_process(value: ExecutionMode) -> ProcessExecutionMode {
    match value {
        ExecutionMode::Argv { program, args } => ProcessExecutionMode::Argv { program, args },
        ExecutionMode::RcShell { script } => ProcessExecutionMode::RcShell { script },
        ExecutionMode::SystemShell { command } => ProcessExecutionMode::SystemShell { command },
        ExecutionMode::SystemLoginShell => ProcessExecutionMode::SystemLoginShell,
    }
}

fn mode_from_process(value: ProcessExecutionMode) -> ExecutionMode {
    match value {
        ProcessExecutionMode::Argv { program, args } => ExecutionMode::Argv { program, args },
        ProcessExecutionMode::RcShell { script } => ExecutionMode::RcShell { script },
        ProcessExecutionMode::SystemShell { command } => ExecutionMode::SystemShell { command },
        ProcessExecutionMode::SystemLoginShell => ExecutionMode::SystemLoginShell,
    }
}

fn environment_to_process(value: EnvironmentSpec) -> ProcessEnvironment {
    ProcessEnvironment {
        base: match value.base {
            EnvironmentBase::Inherit => ProcessEnvironmentBase::Inherit,
            EnvironmentBase::Clean => ProcessEnvironmentBase::Clean,
        },
        changes: value
            .changes
            .into_iter()
            .map(|change| rc_node::ProcessEnvironmentChange {
                name: change.name,
                value: change.value,
            })
            .collect(),
    }
}

fn environment_from_process(value: ProcessEnvironment) -> EnvironmentSpec {
    EnvironmentSpec {
        base: match value.base {
            ProcessEnvironmentBase::Inherit => EnvironmentBase::Inherit,
            ProcessEnvironmentBase::Clean => EnvironmentBase::Clean,
        },
        changes: value
            .changes
            .into_iter()
            .map(|change| rc_protocol::EnvironmentChange {
                name: change.name,
                value: change.value,
            })
            .collect(),
    }
}

fn result(values: Vec<Val>, label: &str) -> Result<Val, String> {
    crate::node::values::result_value(values, label)
}

fn boolean(value: Val) -> Result<bool, String> {
    match value {
        Val::Bool(value) => Ok(value),
        _ => Err("scheduler result is not boolean".into()),
    }
}

fn u64_value(value: Val) -> Result<u64, String> {
    match value {
        Val::U64(value) => Ok(value),
        _ => Err("scheduler value is not u64".into()),
    }
}

fn misfire_name(value: ScheduleMisfirePolicy) -> &'static str {
    match value {
        ScheduleMisfirePolicy::Skip => "skip",
        ScheduleMisfirePolicy::RunOnce => "run-once",
    }
}
