use rc_protocol::{EnvironmentBase, EnvironmentChange, EnvironmentSpec, ExecutionMode};

pub(super) fn execution_mode(
    args: &serde_json::Map<String, serde_json::Value>,
) -> anyhow::Result<ExecutionMode> {
    let argv = args.get("argv").and_then(serde_json::Value::as_array);
    let command = args.get("command").and_then(serde_json::Value::as_str);
    match (argv, command) {
        (Some(_), Some(_)) | (None, None) => {
            anyhow::bail!("exactly one of argv or command is required")
        }
        (Some(values), None) => argv_mode(values),
        (None, Some(source)) => shell_mode(args, source),
    }
}

fn argv_mode(values: &[serde_json::Value]) -> anyhow::Result<ExecutionMode> {
    let values = values
        .iter()
        .map(|value| value.as_str().map(str::to_owned))
        .collect::<Option<Vec<_>>>()
        .ok_or_else(|| anyhow::anyhow!("argv entries must be strings"))?;
    let (program, args) = values
        .split_first()
        .ok_or_else(|| anyhow::anyhow!("argv must contain a program"))?;
    if program.is_empty() {
        anyhow::bail!("argv program must not be empty");
    }
    Ok(ExecutionMode::Argv {
        program: program.clone(),
        args: args.to_vec(),
    })
}

fn shell_mode(
    args: &serde_json::Map<String, serde_json::Value>,
    source: &str,
) -> anyhow::Result<ExecutionMode> {
    if source.trim().is_empty() {
        anyhow::bail!("command must not be empty");
    }
    match args
        .get("shell")
        .and_then(serde_json::Value::as_str)
        .unwrap_or("rc")
    {
        "rc" => Ok(ExecutionMode::RcShell {
            script: source.to_owned(),
        }),
        "system" => Ok(ExecutionMode::SystemShell {
            command: source.to_owned(),
        }),
        _ => anyhow::bail!("shell must be rc or system"),
    }
}

pub(super) fn environment(
    args: &serde_json::Map<String, serde_json::Value>,
) -> anyhow::Result<EnvironmentSpec> {
    let base = match args
        .get("envBase")
        .and_then(serde_json::Value::as_str)
        .unwrap_or("inherit")
    {
        "inherit" => EnvironmentBase::Inherit,
        "clean" => EnvironmentBase::Clean,
        _ => anyhow::bail!("envBase must be inherit or clean"),
    };
    let mut changes = Vec::new();
    if let Some(values) = args.get("env").and_then(serde_json::Value::as_object) {
        for (name, value) in values {
            let value = if value.is_null() {
                None
            } else {
                Some(
                    value
                        .as_str()
                        .ok_or_else(|| {
                            anyhow::anyhow!("environment values must be strings or null")
                        })?
                        .to_owned(),
                )
            };
            changes.push(EnvironmentChange {
                name: name.clone(),
                value,
            });
        }
    }
    Ok(EnvironmentSpec { base, changes })
}
