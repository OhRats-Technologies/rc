use super::{ComponentExecutionManager, ComponentExecutionRuntime, probe_id};
use rc_node::{
    ExecutionManager, ProcessChannel, ProcessEventSink, ProcessExecutionMode, ProcessLifetime,
    ProcessPrincipal, ProcessSpec,
};
use std::{
    sync::Arc,
    time::{Duration, Instant},
};

pub(super) fn check(runtime: ComponentExecutionRuntime) -> anyhow::Result<()> {
    let sink: ProcessEventSink = Arc::new(|_| {});
    let manager = ComponentExecutionManager::new(runtime, sink);
    let executable = std::env::current_exe()?.to_string_lossy().into_owned();
    let mut completed = Vec::new();
    // Each journal permits 4 MiB. The previous reservation budget rejected
    // execution 65 even though these commands produce only a few bytes.
    for _ in 0..80 {
        let id = probe_id("mcp-admission");
        let mut spec = ProcessSpec::command(&id, "unused");
        spec.mode = ProcessExecutionMode::Argv {
            program: executable.clone(),
            args: vec!["text-fixture".into(), "admitted".into()],
        };
        spec.channel = ProcessChannel::Mcp;
        spec.lifetime = ProcessLifetime::Managed;
        spec.principal = ProcessPrincipal {
            user_id: "runtime-check".into(),
            role: "owner".into(),
            can_execute: true,
            can_manage_devices: true,
        };
        spec.user_id = "runtime-check".into();
        spec.authorization_id = "admission-grant".into();
        spec.relay_id = format!("mcp:{id}");
        spec.scrollback_bytes = 4 << 20;
        let duplicate = spec.clone();
        anyhow::ensure!(manager.start(spec)?, "fresh MCP execution was rejected");
        let deadline = Instant::now() + Duration::from_secs(5);
        loop {
            let read = manager
                .execution_read(&id, 0, 1024)
                .ok_or_else(|| anyhow::anyhow!("MCP execution disappeared"))?;
            if read.status == "exited" {
                anyhow::ensure!(read.exit_code == Some(0));
                break;
            }
            anyhow::ensure!(Instant::now() < deadline, "MCP admission probe timed out");
            std::thread::sleep(Duration::from_millis(10));
        }
        anyhow::ensure!(
            !manager.start(duplicate)?,
            "duplicate execution was admitted"
        );
        completed.push(id);
    }
    for id in completed {
        anyhow::ensure!(
            manager.execution_authority(&id)
                == Some((ProcessChannel::Mcp, "admission-grant".into())),
            "admission evicted execution authority"
        );
        let read = manager
            .execution_read(&id, 0, 1024)
            .ok_or_else(|| anyhow::anyhow!("admission evicted completed output"))?;
        let output: Vec<_> = read
            .chunks
            .into_iter()
            .flat_map(|chunk| chunk.bytes)
            .collect();
        anyhow::ensure!(output == b"admitted" && read.status == "exited");
    }
    Ok(())
}
