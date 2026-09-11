use super::{ComponentExecutionRuntime, probe_id};
use rc_node::{
    ProcessChannel, ProcessEnvironment, ProcessExecutionMode, ProcessLifetime, ProcessPrincipal,
    ProcessStartRequest,
};
use std::time::{Duration, Instant};

pub(super) fn check(runtime: ComponentExecutionRuntime) -> anyhow::Result<()> {
    let executable = std::env::current_exe()?
        .to_string_lossy()
        .replace('\'', "'\\''");
    let script =
        format!("'{executable}' text-fixture first && '{executable}' text-fixture second ; exit 7");
    let execution = runtime
        .start(ProcessStartRequest {
            execution_id: probe_id("mcp-completed-read"),
            mode: ProcessExecutionMode::RcShell { script },
            cwd: None,
            environment: ProcessEnvironment::default(),
            terminal: None,
            channel: ProcessChannel::Mcp,
            lifetime: ProcessLifetime::Managed,
            principal: ProcessPrincipal {
                user_id: "runtime-check".into(),
                role: "owner".into(),
                can_execute: true,
                can_manage_devices: true,
            },
            max_runtime_ms: None,
        })
        .map_err(anyhow::Error::msg)?;
    let deadline = Instant::now() + Duration::from_secs(5);
    let mut read = loop {
        let read = execution.read(0, 64 * 1024).map_err(anyhow::Error::msg)?;
        if read.status == "exited" {
            break read;
        }
        anyhow::ensure!(Instant::now() < deadline, "MCP completion probe timed out");
        std::thread::sleep(Duration::from_millis(10));
    };
    for _ in 0..3 {
        let bytes: Vec<_> = read
            .chunks
            .into_iter()
            .flat_map(|chunk| chunk.bytes)
            .collect();
        anyhow::ensure!(bytes == b"firstsecond", "completed MCP output changed");
        anyhow::ensure!(read.status == "exited" && read.exit_code == Some(7));
        read = execution.read(0, 64 * 1024).map_err(anyhow::Error::msg)?;
    }
    Ok(())
}
