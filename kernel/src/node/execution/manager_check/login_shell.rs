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
    wait_for_exit(&rx)
}

fn wait_for_exit(rx: &mpsc::Receiver<ProcessEvent>) -> anyhow::Result<()> {
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
