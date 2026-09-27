use super::{ComponentExecutionManager, ComponentExecutionRuntime, probe_id};
use rc_node::{
    ExecutionManager, ProcessChannel, ProcessEvent, ProcessEventSink, ProcessExecutionMode,
    ProcessLifetime, ProcessPrincipal, ProcessSpec,
};
use std::{
    sync::{Arc, mpsc},
    time::{Duration, Instant},
};

pub(super) fn check(runtime: ComponentExecutionRuntime) -> anyhow::Result<()> {
    let (tx, rx) = mpsc::channel();
    let sink: ProcessEventSink = Arc::new(move |event| {
        let _ = tx.send(event);
    });
    let manager = ComponentExecutionManager::new(runtime, sink);
    let id = probe_id("terminal-input-check");
    let mut spec = ProcessSpec::command(&id, "unused");
    spec.mode = ProcessExecutionMode::Argv {
        program: if cfg!(windows) { "cmd.exe" } else { "/bin/sh" }.into(),
        args: vec![if cfg!(windows) { "/D" } else { "-i" }.into()],
    };
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
    anyhow::ensure!(manager.start(spec)?, "terminal input probe did not start");
    manager.resize(&id, 100, 40)?;
    manager.input(&id, b"echo rc-terminal-probe\rexit 0\r")?;
    let deadline = Instant::now() + Duration::from_secs(10);
    let mut output = Vec::new();
    loop {
        anyhow::ensure!(Instant::now() < deadline, "terminal input probe timed out");
        match rx.recv_timeout(Duration::from_millis(100)) {
            Ok(event @ (ProcessEvent::Stdout { .. } | ProcessEvent::Stderr { .. })) => {
                if let Some((_, bytes)) = event.output_bytes() {
                    output.extend(bytes);
                }
            }
            Ok(ProcessEvent::Exit { exit_code, .. }) => {
                anyhow::ensure!(exit_code == 0, "terminal input probe failed");
                anyhow::ensure!(
                    String::from_utf8_lossy(&output).contains("rc-terminal-probe"),
                    "terminal output was lost"
                );
                return Ok(());
            }
            Ok(_) | Err(mpsc::RecvTimeoutError::Timeout) => {}
            Err(error) => return Err(error.into()),
        }
    }
}
