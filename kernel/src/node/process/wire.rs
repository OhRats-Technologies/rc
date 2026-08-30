use super::super::values;
use rc_node::{
    ProcessChannel, ProcessEnvironment, ProcessEnvironmentBase, ProcessEnvironmentChange,
    ProcessExecutionMode, ProcessLifetime, ProcessSignal,
};
use rc_protocol::TerminalSpec;
use wasmtime::component::Val;

pub fn mode_value(value: ProcessExecutionMode) -> Val {
    match value {
        ProcessExecutionMode::Argv { program, args } => Val::Variant(
            "argv".into(),
            Some(Box::new(Val::Tuple(vec![
                Val::String(program),
                Val::List(args.into_iter().map(Val::String).collect()),
            ]))),
        ),
        ProcessExecutionMode::RcShell { script } => {
            Val::Variant("rc-shell".into(), Some(Box::new(Val::String(script))))
        }
        ProcessExecutionMode::SystemShell { command } => {
            Val::Variant("system-shell".into(), Some(Box::new(Val::String(command))))
        }
        ProcessExecutionMode::SystemLoginShell => Val::Variant("system-login-shell".into(), None),
    }
}

pub fn mode_field(fields: &[(String, Val)], name: &str) -> Result<ProcessExecutionMode, String> {
    match values::field(fields, name)? {
        Val::Variant(kind, Some(payload)) if kind == "argv" => {
            let Val::Tuple(values) = &**payload else {
                return Err("argv mode payload is not a tuple".into());
            };
            let [Val::String(program), Val::List(args)] = values.as_slice() else {
                return Err("argv mode payload is invalid".into());
            };
            let args = args
                .iter()
                .map(|value| match value {
                    Val::String(value) => Ok(value.clone()),
                    _ => Err(String::from("argv argument is not a string")),
                })
                .collect::<Result<Vec<String>, String>>()?;
            Ok(ProcessExecutionMode::Argv {
                program: program.clone(),
                args,
            })
        }
        Val::Variant(kind, Some(payload)) if kind == "rc-shell" => match &**payload {
            Val::String(value) => Ok(ProcessExecutionMode::RcShell {
                script: value.clone(),
            }),
            _ => Err("rc-shell payload is not a string".into()),
        },
        Val::Variant(kind, Some(payload)) if kind == "system-shell" => match &**payload {
            Val::String(value) => Ok(ProcessExecutionMode::SystemShell {
                command: value.clone(),
            }),
            _ => Err("system-shell payload is not a string".into()),
        },
        Val::Variant(kind, None) if kind == "system-login-shell" => {
            Ok(ProcessExecutionMode::SystemLoginShell)
        }
        _ => Err("invalid execution mode".into()),
    }
}

pub fn environment_value(value: ProcessEnvironment) -> Val {
    Val::Record(vec![
        (
            "base".into(),
            Val::Enum(
                match value.base {
                    ProcessEnvironmentBase::Inherit => "inherit",
                    ProcessEnvironmentBase::Clean => "clean",
                }
                .into(),
            ),
        ),
        (
            "changes".into(),
            Val::List(
                value
                    .changes
                    .into_iter()
                    .map(|change| {
                        Val::Record(vec![
                            ("name".into(), Val::String(change.name)),
                            ("value".into(), option_string(change.value)),
                        ])
                    })
                    .collect(),
            ),
        ),
    ])
}

pub fn environment_field(
    fields: &[(String, Val)],
    name: &str,
) -> Result<ProcessEnvironment, String> {
    let record = values::record(values::field(fields, name)?.clone(), name)?;
    let base = match values::enum_field(&record, "base")?.as_str() {
        "inherit" => ProcessEnvironmentBase::Inherit,
        "clean" => ProcessEnvironmentBase::Clean,
        _ => return Err("invalid environment base".into()),
    };
    let changes = values::list_field(&record, "changes")?
        .into_iter()
        .map(|value| {
            let fields = values::record(value, "environment change")?;
            Ok(ProcessEnvironmentChange {
                name: values::string_field(&fields, "name")?,
                value: option_string_field(&fields, "value")?,
            })
        })
        .collect::<Result<_, String>>()?;
    Ok(ProcessEnvironment { base, changes })
}

pub fn option_string(value: Option<String>) -> Val {
    Val::Option(value.map(|value| Box::new(Val::String(value))))
}

pub fn option_string_field(fields: &[(String, Val)], name: &str) -> Result<Option<String>, String> {
    match values::field(fields, name)? {
        Val::Option(None) => Ok(None),
        Val::Option(Some(value)) => match &**value {
            Val::String(value) => Ok(Some(value.clone())),
            _ => Err(format!("field {name:?} is not an optional string")),
        },
        _ => Err(format!("field {name:?} is not an option")),
    }
}

pub fn option_u64(value: Option<u64>) -> Val {
    Val::Option(value.map(|value| Box::new(Val::U64(value))))
}

pub fn option_u64_field(fields: &[(String, Val)], name: &str) -> Result<Option<u64>, String> {
    match values::field(fields, name)? {
        Val::Option(None) => Ok(None),
        Val::Option(Some(value)) => match &**value {
            Val::U64(value) => Ok(Some(*value)),
            _ => Err(format!("field {name:?} is not an optional u64")),
        },
        _ => Err(format!("field {name:?} is not an option")),
    }
}

pub fn terminal_value(value: Option<TerminalSpec>) -> Val {
    Val::Option(value.map(|value| {
        Box::new(Val::Record(vec![
            ("cols".into(), Val::U16(value.cols)),
            ("rows".into(), Val::U16(value.rows)),
            ("term".into(), Val::String(value.term)),
        ]))
    }))
}

pub fn terminal_option(fields: &[(String, Val)]) -> Result<Option<TerminalSpec>, String> {
    values::option_record_field(fields, "terminal")?
        .map(|fields| {
            Ok(TerminalSpec {
                cols: values::u16_field(&fields, "cols")?,
                rows: values::u16_field(&fields, "rows")?,
                term: values::string_field(&fields, "term")?,
            })
        })
        .transpose()
}

pub fn channel_name(value: ProcessChannel) -> &'static str {
    match value {
        ProcessChannel::Control => "control",
        ProcessChannel::Ssh => "ssh",
        ProcessChannel::Mcp => "mcp",
        ProcessChannel::Schedule => "schedule",
    }
}

pub fn lifetime_name(value: ProcessLifetime) -> &'static str {
    match value {
        ProcessLifetime::Attached => "attached",
        ProcessLifetime::Managed => "managed",
        ProcessLifetime::Scheduled => "scheduled",
    }
}

pub fn signal_name(value: ProcessSignal) -> &'static str {
    match value {
        ProcessSignal::Interrupt => "interrupt",
        ProcessSignal::Terminate => "terminate",
        ProcessSignal::Kill => "kill",
    }
}

pub fn parse_signal(value: &str) -> Result<ProcessSignal, String> {
    match value {
        "interrupt" => Ok(ProcessSignal::Interrupt),
        "terminate" => Ok(ProcessSignal::Terminate),
        "kill" => Ok(ProcessSignal::Kill),
        _ => Err("invalid process signal".into()),
    }
}
