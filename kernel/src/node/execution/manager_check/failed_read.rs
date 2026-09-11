use super::{ComponentExecutionRuntime, probe_id};
use rc_node::{
    ProcessChannel, ProcessEnvironment, ProcessExecutionMode, ProcessLifetime, ProcessPrincipal,
    ProcessStartRequest,
};

pub(super) fn check(runtime: ComponentExecutionRuntime) -> anyhow::Result<()> {
    let script = "rc-nonexistent-executable-regression".to_owned();
    let execution = runtime
        .start(ProcessStartRequest {
            execution_id: probe_id("mcp-failed-read"),
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
    let mut first_output = None;
    for _ in 0..3 {
        let read = execution.read(0, 64 * 1024).map_err(anyhow::Error::msg)?;
        anyhow::ensure!(
            read.status == "exited" && read.exit_code == Some(1),
            "shell error was not a completed execution failure"
        );
        let bytes: Vec<_> = read
            .chunks
            .into_iter()
            .flat_map(|chunk| chunk.bytes)
            .collect();
        anyhow::ensure!(String::from_utf8_lossy(&bytes).contains("command not found"));
        if let Some(first) = &first_output {
            anyhow::ensure!(first == &bytes, "failure output changed on repeated read");
        } else {
            first_output = Some(bytes);
        }
    }
    Ok(())
}
