use crate::ohrats::rc_process::{environment_host, process_host::SpawnRequest, types::*};

pub(crate) fn spawn_request(plan: StartPlan) -> Result<SpawnRequest, String> {
    let (program, args) = match plan.mode {
        ExecutionMode::Argv((program, args)) => (program, args),
        ExecutionMode::SystemShell(command) => system_command(command, false)?,
        ExecutionMode::SystemLoginShell => system_command(String::new(), true)?,
        ExecutionMode::RcShell(_) => return Err("RC Shell plan reached native spawn".into()),
    };
    Ok(SpawnRequest {
        program,
        args,
        cwd: plan.cwd,
        environment: plan.environment,
        terminal: plan.terminal,
    })
}

fn system_command(source: String, login: bool) -> Result<(String, Vec<String>), String> {
    let environment = environment_host::snapshot()?;
    let windows = matches!(
        environment_host::host_platform(),
        environment_host::PlatformKind::Windows
    );
    let override_shell = environment
        .iter()
        .find(|(name, value)| name.eq_ignore_ascii_case("RC_SHELL") && !value.trim().is_empty())
        .map(|(_, value)| value.clone());
    let shell = override_shell
        .or_else(|| resolve_default(&environment, windows))
        .ok_or_else(|| "no system shell is available".to_owned())?;
    shell_invocation(shell, source, login, windows)
}

fn shell_invocation(
    shell: String,
    source: String,
    login: bool,
    windows: bool,
) -> Result<(String, Vec<String>), String> {
    let name = shell.rsplit(['/', '\\']).next().unwrap_or(&shell);
    if name.eq_ignore_ascii_case("cmd.exe") || name.eq_ignore_ascii_case("cmd") {
        return Ok((
            shell,
            if login {
                vec!["/D".into()]
            } else {
                vec!["/D".into(), "/S".into(), "/C".into(), source]
            },
        ));
    }
    if name.eq_ignore_ascii_case("pwsh.exe")
        || name.eq_ignore_ascii_case("pwsh")
        || name.eq_ignore_ascii_case("powershell.exe")
    {
        let mut args = vec!["-NoLogo".into()];
        if !login {
            args.extend(["-NonInteractive".into(), "-Command".into(), source]);
        }
        return Ok((shell, args));
    }
    if windows {
        return Err("RC_SHELL on Windows must select cmd.exe, pwsh.exe, or powershell.exe".into());
    }
    Ok((
        shell,
        if login {
            vec!["-l".into()]
        } else {
            vec!["-lc".into(), source]
        },
    ))
}

fn resolve_default(environment: &[(String, String)], windows: bool) -> Option<String> {
    if windows {
        for candidate in ["pwsh.exe", "powershell.exe", "cmd.exe"] {
            if let Ok(Some(path)) = environment_host::find_executable(candidate, environment) {
                return Some(path);
            }
        }
        return None;
    }
    environment
        .iter()
        .find(|(name, value)| name == "SHELL" && !value.trim().is_empty())
        .map(|(_, value)| value.clone())
        .or_else(|| Some("/bin/sh".into()))
}

#[cfg(test)]
mod tests {
    use super::shell_invocation;

    #[test]
    fn maps_native_shell_families_without_portable_shell_parsing() {
        assert_eq!(
            shell_invocation("cmd.exe".into(), "echo hi".into(), false, true)
                .unwrap()
                .1,
            ["/D", "/S", "/C", "echo hi"]
        );
        assert_eq!(
            shell_invocation("pwsh.exe".into(), String::new(), true, true)
                .unwrap()
                .1,
            ["-NoLogo"]
        );
        assert_eq!(
            shell_invocation("/bin/zsh".into(), String::new(), true, false)
                .unwrap()
                .1,
            ["-l"]
        );
        assert!(shell_invocation("custom.exe".into(), String::new(), true, true).is_err());
    }
}
