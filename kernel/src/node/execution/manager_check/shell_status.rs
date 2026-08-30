use super::{ComponentExecutionManager, ComponentExecutionRuntime};
use rc_node::{
    ExecutionManager, ProcessChannel, ProcessEvent, ProcessEventSink, ProcessExecutionMode,
    ProcessLifetime, ProcessPrincipal, ProcessSpec,
};
use std::{
    sync::{Arc, mpsc},
    time::{Duration, Instant},
};

pub(super) fn check_exit(runtime: ComponentExecutionRuntime) -> anyhow::Result<()> {
    let (tx, rx) = mpsc::channel();
    let sink: ProcessEventSink = Arc::new(move |event| {
        let _ = tx.send(event);
    });
    let manager = ComponentExecutionManager::new(runtime, sink);
    let script = "exit 7 ; echo must-not-run";
    let mut spec = ProcessSpec::command("manager-shell-exit-check", script);
    spec.mode = ProcessExecutionMode::RcShell {
        script: script.into(),
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
    anyhow::ensure!(manager.start(spec)?);
    let deadline = Instant::now() + Duration::from_secs(5);
    while Instant::now() < deadline {
        match rx.recv_timeout(Duration::from_millis(100)) {
            Ok(ProcessEvent::Stdout { .. } | ProcessEvent::Stderr { .. }) => {
                anyhow::bail!("portable shell continued after exit")
            }
            Ok(ProcessEvent::Exit { exit_code, .. }) => {
                anyhow::ensure!(exit_code == 7, "portable shell lost explicit exit status");
                return Ok(());
            }
            Ok(ProcessEvent::Started { .. }) | Err(mpsc::RecvTimeoutError::Timeout) => {}
            Err(error) => return Err(error.into()),
        }
    }
    anyhow::bail!("portable shell exit probe timed out")
}
