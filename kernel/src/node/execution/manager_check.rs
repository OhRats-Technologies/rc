use super::{ComponentExecutionManager, ComponentExecutionRuntime};
use rc_node::{
    ExecutionManager, ProcessChannel, ProcessEvent, ProcessEventSink, ProcessExecutionMode,
    ProcessLifetime, ProcessPrincipal, ProcessSpec,
};
use std::{
    sync::{Arc, mpsc},
    time::{Duration, Instant, SystemTime, UNIX_EPOCH},
};
use std::sync::atomic::{AtomicU64, Ordering};
mod cancellation;
mod glob_check;
mod redirect;
mod shell_argv;
mod shell_status;

pub fn check_manager(runtime: ComponentExecutionRuntime) -> anyhow::Result<()> {
    check_exact_argv(runtime.clone())?;
    check_system_login_shell(runtime.clone())?;
    check_portable_shell(runtime)
}

static PROBE_SEQUENCE: AtomicU64 = AtomicU64::new(0);

pub(super) fn probe_id(prefix: &str) -> String {
    let timestamp = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos();
    format!(
        "{prefix}-{timestamp}-{}-{}",
        std::process::id(),
        PROBE_SEQUENCE.fetch_add(1, Ordering::Relaxed)
    )
}

fn check_exact_argv(runtime: ComponentExecutionRuntime) -> anyhow::Result<()> {
    let (tx, rx) = mpsc::channel();
    let sink: ProcessEventSink = Arc::new(move |event| {
        let _ = tx.send(event);
    });
    let manager = ComponentExecutionManager::new(runtime, sink);
    let executable = std::env::current_exe()?.to_string_lossy().into_owned();
    let execution_id = std::env::var("RC_EXECUTION_REPLAY_PROBE_ID")
        .unwrap_or_else(|_| probe_id("manager-check"));
    let mut spec = ProcessSpec::command(&execution_id, "unused");
    spec.mode = ProcessExecutionMode::Argv {
        program: executable,
        args: vec!["--version".into()],
    };
    spec.channel = ProcessChannel::Control;
    spec.lifetime = ProcessLifetime::Managed;
    spec.principal = ProcessPrincipal {
        user_id: "runtime-check".into(),
        role: "owner".into(),
        can_execute: true,
        can_manage_devices: true,
    };
    spec.user_id = "runtime-check".into();
    spec.authorization_id = "grant-a".into();
    spec.scrollback_bytes = 64 * 1024;
    let duplicate = spec.clone();
    anyhow::ensure!(
        manager.start(spec)?,
        "manager rejected a fresh execution id"
    );
    anyhow::ensure!(
        manager.execution_authority(&execution_id)
            == Some((ProcessChannel::Control, "grant-a".into())),
        "execution did not retain its authorization linkage"
    );
    anyhow::ensure!(
        !manager.start(duplicate)?,
        "manager accepted a duplicate execution id"
    );
    let deadline = Instant::now() + Duration::from_secs(5);
    let mut output = Vec::new();
    let mut exited = false;
    while Instant::now() < deadline {
        let event = rx.recv_timeout(Duration::from_millis(100));
        match event {
            Ok(event @ (ProcessEvent::Stdout { .. } | ProcessEvent::Stderr { .. })) => {
                if let Some((_, bytes)) = event.output_bytes() {
                    output.extend(bytes);
                }
            }
            Ok(ProcessEvent::Exit { exit_code, .. }) => {
                anyhow::ensure!(exit_code == 0);
                exited = true;
                break;
            }
            Ok(ProcessEvent::Started { .. }) | Err(mpsc::RecvTimeoutError::Timeout) => {}
            Err(error) => return Err(error.into()),
        }
    }
    anyhow::ensure!(
        exited && String::from_utf8_lossy(&output).starts_with("RC kernel "),
        "component execution manager probe failed"
    );
    Ok(())
}

fn check_system_login_shell(runtime: ComponentExecutionRuntime) -> anyhow::Result<()> {
    let (tx, rx) = mpsc::channel();
    let sink: ProcessEventSink = Arc::new(move |event| {
        let _ = tx.send(event);
    });
    let manager = ComponentExecutionManager::new(runtime, sink);
    let id = probe_id("manager-login-shell-check");
    let mut spec = ProcessSpec::command(&id, "unused");
    spec.mode = ProcessExecutionMode::SystemLoginShell;
    spec.terminal = Some(rc_protocol::TerminalSpec {
        cols: 80,
        rows: 24,
        term: "xterm-256color".into(),
    });
    spec.channel = ProcessChannel::Control;
    spec.lifetime = ProcessLifetime::Managed;
    spec.principal = ProcessPrincipal {
        user_id: "runtime-check".into(),
        role: "owner".into(),
        can_execute: true,
        can_manage_devices: true,
    };
    spec.user_id = "runtime-check".into();
    spec.scrollback_bytes = 64 * 1024;
    anyhow::ensure!(manager.start(spec)?, "system login shell did not start");
    manager.signal(&id, "KILL")?;
    let deadline = Instant::now() + Duration::from_secs(8);
    while Instant::now() < deadline {
        match rx.recv_timeout(Duration::from_millis(100)) {
            Ok(ProcessEvent::Exit { .. }) => return Ok(()),
            Ok(_) | Err(mpsc::RecvTimeoutError::Timeout) => {}
            Err(error) => return Err(error.into()),
        }
    }
    anyhow::bail!("system login shell probe timed out")
}

fn check_portable_shell(runtime: ComponentExecutionRuntime) -> anyhow::Result<()> {
    let mut spec = ProcessSpec::command("manager-shell-check", "echo portable-shell");
    spec.mode = ProcessExecutionMode::RcShell {
        script: "echo portable-shell".into(),
    };
    let output = run_and_collect(runtime.clone(), spec)?;
    anyhow::ensure!(
        output == b"portable-shell\n",
        "portable shell manager probe returned unexpected output"
    );
    shell_argv::check(runtime.clone())?;
    glob_check::check(runtime.clone())?;
    shell_status::check_exit(runtime.clone())?;
    let script = "false && echo no || echo yes ; echo done";
    let mut connectors = ProcessSpec::command("manager-shell-connectors-check", script);
    connectors.mode = ProcessExecutionMode::RcShell {
        script: script.into(),
    };
    anyhow::ensure!(
        run_and_collect(runtime.clone(), connectors)? == b"yes\ndone\n",
        "portable shell connector probe returned unexpected output"
    );
    let script = "FOO=one ; export BAR=two ; echo $FOO $BAR ; unset FOO ; echo $FOO";
    let mut environment = ProcessSpec::command("manager-shell-environment-check", script);
    environment.mode = ProcessExecutionMode::RcShell {
        script: script.into(),
    };
    anyhow::ensure!(
        run_and_collect(runtime.clone(), environment)? == b"one two\n\n",
        "portable shell environment probe returned unexpected output"
    );
    let script = "Path=lower ; PATH=upper ; echo $Path $PATH";
    let mut environment_case = ProcessSpec::command("manager-shell-env-case-check", script);
    environment_case.mode = ProcessExecutionMode::RcShell {
        script: script.into(),
    };
    let expected: &[u8] = if cfg!(windows) {
        b"upper upper\n"
    } else {
        b"lower upper\n"
    };
    anyhow::ensure!(
        run_and_collect(runtime.clone(), environment_case)? == expected,
        "portable shell environment case probe returned unexpected output"
    );
    let script = "echo portable-builtin | cat";
    let mut builtins = ProcessSpec::command("manager-shell-builtin-pipeline-check", script);
    builtins.mode = ProcessExecutionMode::RcShell {
        script: script.into(),
    };
    anyhow::ensure!(
        run_and_collect(runtime.clone(), builtins)? == b"portable-builtin\n",
        "portable builtin pipeline probe returned unexpected output"
    );
    let script = "echo $(echo nested)";
    let mut substitution = ProcessSpec::command("manager-shell-substitution-check", script);
    substitution.mode = ProcessExecutionMode::RcShell {
        script: script.into(),
    };
    anyhow::ensure!(
        run_and_collect(runtime.clone(), substitution)? == b"nested\n",
        "portable shell substitution probe returned unexpected output"
    );
    let script = "echo $(echo nested | cat)";
    let mut pipeline_substitution =
        ProcessSpec::command("manager-shell-pipeline-substitution-check", script);
    pipeline_substitution.mode = ProcessExecutionMode::RcShell {
        script: script.into(),
    };
    anyhow::ensure!(
        run_and_collect(runtime.clone(), pipeline_substitution)? == b"nested\n",
        "portable shell pipeline substitution probe returned unexpected output"
    );
    let script = "echo $(echo $(echo deep))";
    let mut nested_substitution =
        ProcessSpec::command("manager-shell-nested-substitution-check", script);
    nested_substitution.mode = ProcessExecutionMode::RcShell {
        script: script.into(),
    };
    anyhow::ensure!(
        run_and_collect(runtime.clone(), nested_substitution)? == b"deep\n",
        "portable shell nested substitution probe returned unexpected output"
    );
    let script = "seq 2 2 6";
    let mut sequence = ProcessSpec::command("manager-shell-seq-check", script);
    sequence.mode = ProcessExecutionMode::RcShell {
        script: script.into(),
    };
    anyhow::ensure!(
        run_and_collect(runtime.clone(), sequence)? == b"2\n4\n6\n",
        "portable shell seq probe returned unexpected output"
    );
    cancellation::check_pipeline_cancel(runtime.clone())?;
    redirect::check(runtime.clone())?;
    #[cfg(unix)]
    {
        let mut pipeline = ProcessSpec::command(
            "manager-shell-pipeline-check",
            "printf portable-pipeline | cat",
        );
        pipeline.mode = ProcessExecutionMode::RcShell {
            script: "printf portable-pipeline | cat".into(),
        };
        anyhow::ensure!(
            run_and_collect(runtime.clone(), pipeline)? == b"portable-pipeline",
            "portable shell pipeline probe returned unexpected output"
        );
    }
    Ok(())
}

fn run_and_collect(
    runtime: ComponentExecutionRuntime,
    mut spec: ProcessSpec,
) -> anyhow::Result<Vec<u8>> {
    spec.id = probe_id(&spec.id);
    let execution_id = spec.id.clone();
    let (tx, rx) = mpsc::channel();
    let sink: ProcessEventSink = Arc::new(move |event| {
        let _ = tx.send(event);
    });
    let manager = ComponentExecutionManager::new(runtime, sink);
    spec.channel = ProcessChannel::Control;
    spec.lifetime = ProcessLifetime::Managed;
    spec.principal = ProcessPrincipal {
        user_id: "runtime-check".into(),
        role: "owner".into(),
        can_execute: true,
        can_manage_devices: true,
    };
    spec.user_id = "runtime-check".into();
    spec.scrollback_bytes = 64 * 1024;
    anyhow::ensure!(
        manager
            .start(spec)
            .map_err(|error| anyhow::anyhow!("execution {execution_id}: {error}"))?,
        "manager rejected a fresh execution id"
    );
    let deadline = Instant::now() + Duration::from_secs(5);
    let mut output = Vec::new();
    while Instant::now() < deadline {
        match rx.recv_timeout(Duration::from_millis(100)) {
            Ok(event @ (ProcessEvent::Stdout { .. } | ProcessEvent::Stderr { .. })) => {
                if let Some((_, bytes)) = event.output_bytes() {
                    output.extend(bytes);
                }
            }
            Ok(ProcessEvent::Exit { exit_code, .. }) => {
                anyhow::ensure!(exit_code == 0);
                return Ok(output);
            }
            Ok(ProcessEvent::Started { .. }) | Err(mpsc::RecvTimeoutError::Timeout) => {}
            Err(error) => return Err(error.into()),
        }
    }
    anyhow::bail!("component execution manager probe timed out")
}
