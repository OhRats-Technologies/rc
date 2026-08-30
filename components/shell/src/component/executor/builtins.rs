use super::{Output, StreamKind, VirtualJob, sequence::Context};
use crate::component::ohrats::rc_process::{environment_host, filesystem_host};

const NAMES: &[&str] = &[
    "cd", "pwd", "echo", "true", "false", "env", "export", "unset", "dirname", "basename", "exit",
    "which", "cat", "touch", "mkdir", "rm", "mv", "ls", "yes", "seq",
];
type BuiltinResult = Result<(Option<Vec<u8>>, u32), String>;

pub(super) fn run(argv: &[String], context: &mut Context) -> Option<BuiltinResult> {
    let command = argv.first()?.as_str();
    if !NAMES.contains(&command) {
        return None;
    }
    Some(match command {
        "echo" => Ok((Some(format!("{}\n", argv[1..].join(" ")).into_bytes()), 0)),
        "pwd" => Ok((
            Some(format!("{}\n", context.cwd.as_deref().unwrap_or(".")).into_bytes()),
            0,
        )),
        "true" => Ok((None, 0)),
        "false" => Ok((None, 1)),
        "env" => Ok((Some(render_environment(&context.environment)), 0)),
        "export" => export(&argv[1..], context).map(|_| (None, 0)),
        "unset" => {
            for name in &argv[1..] {
                remove(
                    &mut context.environment,
                    name,
                    context.case_insensitive_environment,
                );
            }
            Ok((None, 0))
        }
        "cd" => change_directory(argv.get(1).map(String::as_str), context).map(|_| (None, 0)),
        "dirname" => Ok((Some(format!("{}\n", dirname(argv.get(1))).into_bytes()), 0)),
        "basename" => Ok((Some(format!("{}\n", basename(argv.get(1))).into_bytes()), 0)),
        "exit" => argv.get(1).map_or(Ok((None, 0)), |value| {
            value
                .parse::<u32>()
                .map(|code| (None, code))
                .map_err(|_| "exit status must be an unsigned integer".into())
        }),
        "which" => which(&argv[1..], context).map(|bytes| (Some(bytes), 0)),
        "cat" => cat(&argv[1..], context).map(|bytes| (Some(bytes), 0)),
        "touch" => touch(&argv[1..], context).map(|_| (None, 0)),
        "mkdir" => mkdir(&argv[1..], context).map(|_| (None, 0)),
        "rm" => remove_paths(&argv[1..], context).map(|_| (None, 0)),
        "mv" => move_path(&argv[1..], context).map(|_| (None, 0)),
        "ls" => list(argv.get(1).map(String::as_str), context).map(|bytes| (Some(bytes), 0)),
        "yes" => Ok((None, 0)),
        "seq" => sequence(&argv[1..]).map(|bytes| (Some(bytes), 0)),
        _ => return None,
    })
}

fn sequence(values: &[String]) -> Result<Vec<u8>, String> {
    let numbers = values
        .iter()
        .map(|value| {
            value
                .parse::<i64>()
                .map_err(|_| "seq arguments must be integers".into())
        })
        .collect::<Result<Vec<_>, String>>()?;
    let (mut current, step, end) = match numbers.as_slice() {
        [end] => (1, 1, *end),
        [start, end] => (*start, 1, *end),
        [start, step, end] if *step != 0 => (*start, *step, *end),
        _ => return Err("seq requires LAST, FIRST LAST, or FIRST STEP LAST".into()),
    };
    let mut output = Vec::new();
    for _ in 0..1_000_000 {
        if (step > 0 && current > end) || (step < 0 && current < end) {
            return Ok(output);
        }
        output.extend_from_slice(current.to_string().as_bytes());
        output.push(b'\n');
        current = current.checked_add(step).ok_or("seq range overflow")?;
    }
    Err("seq output exceeds builtin capacity".into())
}

fn which(values: &[String], context: &Context) -> Result<Vec<u8>, String> {
    let mut output = Vec::new();
    for value in values {
        let found = environment_host::find_executable(value, &context.environment)?
            .ok_or_else(|| format!("command not found: {value}"))?;
        output.extend_from_slice(found.as_bytes());
        output.push(b'\n');
    }
    Ok(output)
}

fn cat(values: &[String], context: &Context) -> Result<Vec<u8>, String> {
    let mut output = Vec::new();
    for value in values {
        output.extend(filesystem_host::read(
            &path(context, value),
            64 * 1024 * 1024,
        )?);
    }
    Ok(output)
}

fn touch(values: &[String], context: &Context) -> Result<(), String> {
    for value in values {
        filesystem_host::write(&path(context, value), &[], true)?;
    }
    Ok(())
}

fn mkdir(values: &[String], context: &Context) -> Result<(), String> {
    let recursive = values.iter().any(|value| value == "-p");
    for value in values.iter().filter(|value| !value.starts_with('-')) {
        filesystem_host::create_directory(&path(context, value), recursive)?;
    }
    Ok(())
}

fn remove_paths(values: &[String], context: &Context) -> Result<(), String> {
    let recursive = values
        .iter()
        .any(|value| matches!(value.as_str(), "-r" | "-R" | "-rf" | "-fr"));
    for value in values.iter().filter(|value| !value.starts_with('-')) {
        filesystem_host::remove(&path(context, value), recursive)?;
    }
    Ok(())
}

fn move_path(values: &[String], context: &Context) -> Result<(), String> {
    let [source, destination] = values else {
        return Err("mv requires source and destination".into());
    };
    filesystem_host::rename(&path(context, source), &path(context, destination))
}

fn list(value: Option<&str>, context: &Context) -> Result<Vec<u8>, String> {
    let mut entries =
        filesystem_host::list_directory(&path(context, value.unwrap_or(".")), 16_384)?;
    entries.sort_by(|left, right| left.name.cmp(&right.name));
    Ok(entries
        .into_iter()
        .flat_map(|entry| {
            let mut value = entry.name.into_bytes();
            value.push(b'\n');
            value
        })
        .collect())
}

fn path(context: &Context, value: &str) -> String {
    crate::paths::resolve(
        context.cwd.as_deref(),
        value,
        context.case_insensitive_environment,
    )
}

fn export(values: &[String], context: &mut Context) -> Result<(), String> {
    for value in values {
        let (name, new_value) = value
            .split_once('=')
            .map_or((value.as_str(), None), |(name, value)| (name, Some(value)));
        if name.is_empty() {
            return Err("export name is empty".into());
        }
        let current = new_value
            .map(str::to_owned)
            .or_else(|| {
                lookup(
                    &context.environment,
                    name,
                    context.case_insensitive_environment,
                )
            })
            .unwrap_or_default();
        set(
            &mut context.environment,
            name,
            current,
            context.case_insensitive_environment,
        );
    }
    Ok(())
}

fn change_directory(requested: Option<&str>, context: &mut Context) -> Result<(), String> {
    let requested = requested
        .map(str::to_owned)
        .or_else(|| {
            lookup(
                &context.environment,
                "HOME",
                context.case_insensitive_environment,
            )
        })
        .ok_or("cd requires a path when HOME is unset")?;
    let resolved = path(context, &requested);
    filesystem_host::list_directory(&resolved, 1)?;
    context.cwd = Some(resolved);
    Ok(())
}

fn render_environment(values: &[(String, String)]) -> Vec<u8> {
    let mut output = Vec::new();
    for (name, value) in values {
        output.extend_from_slice(name.as_bytes());
        output.push(b'=');
        output.extend_from_slice(value.as_bytes());
        output.push(b'\n');
    }
    output
}

fn lookup(values: &[(String, String)], name: &str, case_insensitive: bool) -> Option<String> {
    values
        .iter()
        .find(|(candidate, _)| same_name(candidate, name, case_insensitive))
        .map(|(_, value)| value.clone())
}
fn set(values: &mut Vec<(String, String)>, name: &str, value: String, case_insensitive: bool) {
    remove(values, name, case_insensitive);
    values.push((name.into(), value));
}
fn remove(values: &mut Vec<(String, String)>, name: &str, case_insensitive: bool) {
    values.retain(|(candidate, _)| !same_name(candidate, name, case_insensitive));
}
fn same_name(left: &str, right: &str, case_insensitive: bool) -> bool {
    if case_insensitive {
        left.eq_ignore_ascii_case(right)
    } else {
        left == right
    }
}
fn dirname(value: Option<&String>) -> String {
    value
        .and_then(|value| value.rsplit_once(['/', '\\']).map(|(dir, _)| dir))
        .filter(|value| !value.is_empty())
        .unwrap_or(".")
        .into()
}
fn basename(value: Option<&String>) -> String {
    value
        .map(|value| value.trim_end_matches(['/', '\\']))
        .and_then(|value| value.rsplit(['/', '\\']).next())
        .unwrap_or("")
        .into()
}

pub(super) fn output(job: &mut VirtualJob) -> Result<Vec<Output>, String> {
    let Some(bytes) = job.output.take() else {
        return Ok(Vec::new());
    };
    if let Some(target) = &mut job.stdout_target {
        filesystem_host::write(&target.path, &bytes, target.append)?;
        target.written = true;
        Ok(Vec::new())
    } else {
        Ok(vec![Output {
            kind: StreamKind::Stdout,
            bytes,
        }])
    }
}
