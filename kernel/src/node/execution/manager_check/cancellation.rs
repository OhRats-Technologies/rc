use super::{ComponentExecutionManager, ComponentExecutionRuntime, probe_id};
use rc_node::{
    ExecutionManager, ProcessChannel, ProcessEvent, ProcessEventSink, ProcessExecutionMode,
    ProcessLifetime, ProcessPrincipal, ProcessSpec,
};
use std::{
    sync::{Arc, mpsc},
    time::{Duration, Instant},
};

pub(super) fn check_pipeline_cancel(runtime: ComponentExecutionRuntime) -> anyhow::Result<()> {
    let (tx, rx) = mpsc::channel();
    let sink: ProcessEventSink = Arc::new(move |event| {
        let _ = tx.send(event);
    });
    let manager = ComponentExecutionManager::new(runtime, sink);
    let script = "yes portable | cat";
    let id = probe_id("manager-shell-cancel-check");
    let mut spec = ProcessSpec::command(&id, script);
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
    wait_for_output(&rx)?;
    manager.signal(&id, "KILL")?;
    wait_for_exit(&rx)
}

fn wait_for_output(rx: &mpsc::Receiver<ProcessEvent>) -> anyhow::Result<()> {
    let deadline = Instant::now() + Duration::from_secs(5);
    loop {
        anyhow::ensure!(
            Instant::now() < deadline,
            "portable yes producer did not emit"
        );
        if let Ok(event @ ProcessEvent::Stdout { .. }) = rx.recv_timeout(Duration::from_millis(100))
        {
            let (_, data) = event.output_bytes().expect("stdout decodes");
            anyhow::ensure!(data.starts_with(b"portable\n"));
            return Ok(());
        }
    }
}

fn wait_for_exit(rx: &mpsc::Receiver<ProcessEvent>) -> anyhow::Result<()> {
    let deadline = Instant::now() + Duration::from_secs(5);
    while Instant::now() < deadline {
        match rx.recv_timeout(Duration::from_millis(100)) {
            Ok(ProcessEvent::Exit { .. }) => return Ok(()),
            Ok(_) | Err(mpsc::RecvTimeoutError::Timeout) => {}
            Err(error) => return Err(error.into()),
        }
    }
    anyhow::bail!("portable shell pipeline cancellation did not exit")
}
