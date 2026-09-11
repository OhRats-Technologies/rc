use super::{ComponentExecutionRuntime, probe_id};
use rc_node::{
    ProcessChannel, ProcessEnvironment, ProcessExecutionMode, ProcessLifetime, ProcessPrincipal,
    ProcessStartRequest,
};
use std::time::{Duration, Instant};

pub(super) fn check(runtime: ComponentExecutionRuntime) -> anyhow::Result<()> {
    let script = "/bin/sh -c 'echo $$'".to_owned();
    let execution = runtime
        .start(ProcessStartRequest {
            execution_id: probe_id("mcp-reaped-read"),
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
    let read = loop {
        let read = execution.read(0, 64 * 1024).map_err(anyhow::Error::msg)?;
        if read.status == "exited" {
            break read;
        }
        anyhow::ensure!(Instant::now() < deadline, "MCP completion probe timed out");
        std::thread::sleep(Duration::from_millis(10));
    };
    let bytes: Vec<_> = read
        .chunks
        .into_iter()
        .flat_map(|chunk| chunk.bytes)
        .collect();
    let pid: i32 = std::str::from_utf8(&bytes)?.trim().parse()?;
    anyhow::ensure!(unsafe { libc::kill(pid, 0) } == -1);
    anyhow::ensure!(
        std::io::Error::last_os_error().raw_os_error() == Some(libc::ESRCH),
        "completed execution retained its native child"
    );
    anyhow::ensure!(
        execution
            .read(0, 64 * 1024)
            .map_err(anyhow::Error::msg)?
            .status
            == "exited"
    );
    Ok(())
}
