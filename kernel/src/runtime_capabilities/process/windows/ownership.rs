use super::*;

fn marker(name: &str) -> std::path::PathBuf {
    std::env::temp_dir().join(format!(
        "rc-{name}-{}-{}",
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos()
    ))
}

fn invalidate_job(group: &mut Group) {
    unsafe { CloseHandle(group.job) }.unwrap();
    group.job = HANDLE::default();
}

fn powershell(script: &str, terminal: Option<Terminal>) -> SpawnRequest {
    SpawnRequest {
        program: "powershell.exe".into(),
        args: [
            "-NoLogo",
            "-NoProfile",
            "-NonInteractive",
            "-Command",
            script,
        ]
        .into_iter()
        .map(str::to_owned)
        .collect(),
        cwd: None,
        environment: Environment {
            base: EnvironmentBase::Inherit,
            changes: Vec::new(),
        },
        terminal,
    }
}

#[test]
fn failed_job_assignment_does_not_leave_the_child_running() {
    let marker = marker("unowned-child");
    let script = format!(
        "Start-Sleep -Milliseconds 500;[IO.File]::WriteAllText('{}','leaked')",
        marker.to_string_lossy().replace('\'', "''")
    );
    let mut group = Group::new().unwrap();
    invalidate_job(&mut group);
    assert!(spawn(&mut group, powershell(&script, None)).is_err());
    std::thread::sleep(std::time::Duration::from_millis(750));
    assert!(!marker.exists(), "child survived failed Job assignment");
}

#[test]
fn conpty_target_cannot_start_before_job_assignment() {
    let marker = marker("unowned-conpty-child");
    let script = format!(
        "[IO.File]::WriteAllText('{}','leaked')",
        marker.to_string_lossy().replace('\'', "''")
    );
    let mut group = Group::new().unwrap();
    invalidate_job(&mut group);
    let terminal = Terminal {
        cols: 80,
        rows: 24,
        term: "xterm-256color".into(),
    };
    assert!(spawn(&mut group, powershell(&script, Some(terminal))).is_err());
    std::thread::sleep(std::time::Duration::from_millis(250));
    assert!(
        !marker.exists(),
        "ConPTY target escaped before Job assignment"
    );
}
