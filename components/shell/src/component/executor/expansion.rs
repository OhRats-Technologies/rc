use crate::component::ohrats::rc_process::{
    environment_host, filesystem_host,
    types::{Environment, EnvironmentBase, EnvironmentChange},
};
use crate::{Assignment, Command, ExpansionHost, RedirectMode, RedirectStream, expand_word};
use std::collections::BTreeMap;

pub(super) type EnvironmentValues = Vec<(String, String)>;

pub(super) fn case_insensitive_environment() -> Result<bool, String> {
    Ok(matches!(
        environment_host::host_platform(),
        environment_host::PlatformKind::Windows
    ))
}

pub(super) fn environment(
    spec: Environment,
    case_insensitive: bool,
) -> Result<EnvironmentValues, String> {
    let mut values = if matches!(spec.base, EnvironmentBase::Inherit) {
        environment_host::snapshot()?
    } else {
        Vec::new()
    };
    if case_insensitive {
        let mut names = std::collections::BTreeSet::new();
        for change in &spec.changes {
            if !names.insert(change.name.to_ascii_uppercase()) {
                return Err("conflicting Windows environment changes".into());
            }
        }
    }
    for change in spec.changes {
        if let Some(index) = values
            .iter()
            .position(|(name, _)| same_name(name, &change.name, case_insensitive))
        {
            if let Some(value) = change.value {
                values[index] = (change.name, value);
            } else {
                values.remove(index);
            }
        } else if let Some(value) = change.value {
            values.push((change.name, value));
        }
    }
    Ok(values)
}

pub(super) fn expand_command(
    command: &Command,
    environment: &[(String, String)],
    cwd: Option<&str>,
    case_insensitive: bool,
) -> Result<(Vec<String>, EnvironmentValues), String> {
    let mut host = Host {
        environment: environment_map(environment, case_insensitive),
        cwd: cwd.unwrap_or("."),
        case_insensitive,
    };
    let mut changes = Vec::new();
    for Assignment { name, value } in &command.assignments {
        let value = expand_word(value, &mut host)
            .map_err(expansion_error)?
            .join(" ");
        host.environment
            .insert(environment_key(name, case_insensitive), value.clone());
        changes.push((name.clone(), value));
    }
    let mut argv = Vec::new();
    for word in &command.words {
        argv.extend(expand_word(word, &mut host).map_err(expansion_error)?);
    }
    Ok((argv, changes))
}

#[derive(Default)]
pub(super) struct Redirects {
    pub stdin: Option<Vec<u8>>,
    pub stdout: Option<(String, bool)>,
    pub stderr: Option<(String, bool)>,
}

pub(super) fn redirects(
    command: &Command,
    environment: &[(String, String)],
    cwd: Option<&str>,
    case_insensitive: bool,
) -> Result<Redirects, String> {
    let mut host = Host {
        environment: environment_map(environment, case_insensitive),
        cwd: cwd.unwrap_or("."),
        case_insensitive,
    };
    let mut result = Redirects::default();
    for redirect in &command.redirects {
        let target = expand_word(&redirect.target, &mut host)
            .map_err(expansion_error)?
            .into_iter()
            .next()
            .ok_or("redirect target is empty")?;
        let path = join_path(cwd, &target);
        if matches!(redirect.mode, RedirectMode::Read) {
            result.stdin = Some(filesystem_host::read(&path, 64 * 1024 * 1024)?);
            continue;
        }
        let value = (path, matches!(redirect.mode, RedirectMode::Append));
        match redirect.stream {
            RedirectStream::Stdout => result.stdout = Some(value),
            RedirectStream::Stderr => result.stderr = Some(value),
            RedirectStream::StdoutAndStderr => {
                result.stdout = Some(value.clone());
                result.stderr = Some(value);
            }
            RedirectStream::Stdin => return Err("stdin redirect must use read mode".into()),
        }
    }
    Ok(result)
}

pub(super) fn resolve_program(
    program: &str,
    environment: &[(String, String)],
) -> Result<String, String> {
    if program.contains(['/', '\\']) {
        return Ok(program.into());
    }
    environment_host::find_executable(program, environment)?
        .ok_or_else(|| format!("command not found: {program}"))
}

pub(super) fn host_environment(values: Vec<(String, String)>) -> Environment {
    Environment {
        base: EnvironmentBase::Clean,
        changes: values
            .into_iter()
            .map(|(name, value)| EnvironmentChange {
                name,
                value: Some(value),
            })
            .collect(),
    }
}

struct Host<'a> {
    environment: BTreeMap<String, String>,
    cwd: &'a str,
    case_insensitive: bool,
}

impl ExpansionHost for Host<'_> {
    fn environment(&self, name: &str) -> Option<String> {
        self.environment
            .get(&environment_key(name, self.case_insensitive))
            .cloned()
    }
    fn command_substitution(&mut self, source: &str) -> Result<Vec<u8>, String> {
        Err(format!(
            "command substitution {source:?} reached expansion before preparation"
        ))
    }
    fn glob(&self, pattern: &str) -> Result<Vec<String>, String> {
        let (directory, name, prefix) = split_pattern(self.cwd, pattern);
        let mut values = filesystem_host::list_directory(&directory, 16_384)?
            .into_iter()
            .filter(|entry| wildcard(name, &entry.name, self.case_insensitive))
            .map(|entry| format!("{prefix}{}", entry.name))
            .collect::<Vec<_>>();
        values.sort();
        Ok(values)
    }
}

fn split_pattern<'a>(cwd: &'a str, pattern: &'a str) -> (String, &'a str, &'a str) {
    if pattern.as_bytes().get(1) == Some(&b':') && !pattern[2..].contains(['/', '\\']) {
        return (pattern[..2].into(), &pattern[2..], &pattern[..2]);
    }
    pattern.rfind(['/', '\\']).map_or_else(
        || (cwd.into(), pattern, ""),
        |at| {
            let directory = if at == 0 || (at == 2 && pattern.as_bytes().get(1) == Some(&b':')) {
                pattern[..=at].into()
            } else {
                pattern[..at].into()
            };
            (directory, &pattern[at + 1..], &pattern[..=at])
        },
    )
}

fn wildcard(pattern: &str, value: &str, case_insensitive: bool) -> bool {
    let (mut p, mut v, mut star, mut retry) = (0, 0, None, 0);
    let pbytes = pattern.as_bytes();
    let vbytes = value.as_bytes();
    while v < vbytes.len() {
        if p < pbytes.len()
            && (pbytes[p] == b'?'
                || pbytes[p] == vbytes[v]
                || (case_insensitive && pbytes[p].eq_ignore_ascii_case(&vbytes[v])))
        {
            p += 1;
            v += 1;
        } else if p < pbytes.len() && pbytes[p] == b'*' {
            star = Some(p);
            p += 1;
            retry = v;
        } else if let Some(at) = star {
            p = at + 1;
            retry += 1;
            v = retry;
        } else {
            return false;
        }
    }
    pbytes[p..].iter().all(|byte| *byte == b'*')
}

fn join(directory: &str, name: &str) -> String {
    if directory == "." {
        name.into()
    } else {
        format!("{directory}/{name}")
    }
}
fn join_path(cwd: Option<&str>, path: &str) -> String {
    if path.starts_with('/') || path.starts_with('\\') || path.as_bytes().get(1) == Some(&b':') {
        path.into()
    } else {
        join(cwd.unwrap_or("."), path)
    }
}
pub(super) fn apply_changes(
    values: &mut EnvironmentValues,
    changes: EnvironmentValues,
    case_insensitive: bool,
) {
    for (name, value) in changes {
        values.retain(|(candidate, _)| !same_name(candidate, &name, case_insensitive));
        values.push((name, value));
    }
}
fn environment_map(
    values: &[(String, String)],
    case_insensitive: bool,
) -> BTreeMap<String, String> {
    values
        .iter()
        .map(|(name, value)| (environment_key(name, case_insensitive), value.clone()))
        .collect()
}
fn environment_key(value: &str, case_insensitive: bool) -> String {
    if case_insensitive {
        value.to_ascii_uppercase()
    } else {
        value.into()
    }
}
fn same_name(left: &str, right: &str, case_insensitive: bool) -> bool {
    if case_insensitive {
        left.eq_ignore_ascii_case(right)
    } else {
        left == right
    }
}
fn expansion_error(error: crate::ExpandError) -> String {
    format!("shell expansion failed: {error:?}")
}
