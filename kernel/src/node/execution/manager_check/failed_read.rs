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
    let mut first_error = None;
    for _ in 0..3 {
        let error = execution
            .read(0, 64 * 1024)
            .err()
            .ok_or_else(|| anyhow::anyhow!("missing executable unexpectedly succeeded"))?;
        anyhow::ensure!(
            !error.contains("wasm trap"),
            "failed shell trapped on repeated read"
        );
        if let Some(first) = &first_error {
            anyhow::ensure!(
                first == &error,
                "failed shell error changed on repeated read"
            );
        } else {
            first_error = Some(error);
        }
    }
    Ok(())
}
