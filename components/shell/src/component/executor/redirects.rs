use super::expansion::*;
use crate::component::ohrats::rc_process::{filesystem_host, types::StreamKind};
use crate::{Command, RedirectMode, RedirectStream, expand::expand_word};
#[derive(Default)]
pub(super) struct Redirects {
    pub stdin: Option<Vec<u8>>,
    pub stdout: Option<(String, bool)>,
    pub stderr: Option<(String, bool)>,
    pub stdout_kind: Option<StreamKind>,
    pub stderr_kind: Option<StreamKind>,
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
        if matches!(redirect.mode, RedirectMode::Duplicate) {
            let (file, kind) = match target.as_str() {
                "1" => (
                    result.stdout.clone(),
                    result.stdout_kind.unwrap_or(StreamKind::Stdout),
                ),
                "2" => (
                    result.stderr.clone(),
                    result.stderr_kind.unwrap_or(StreamKind::Stderr),
                ),
                _ => return Err("output duplication requires descriptor 1 or 2".into()),
            };
            match redirect.stream {
                RedirectStream::Stdout => {
                    result.stdout = file;
                    result.stdout_kind = Some(kind);
                }
                RedirectStream::Stderr => {
                    result.stderr = file;
                    result.stderr_kind = Some(kind);
                }
                _ => return Err("only stdout/stderr duplication is supported".into()),
            }
            continue;
        }
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
