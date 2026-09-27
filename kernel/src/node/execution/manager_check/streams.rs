use super::{ComponentExecutionRuntime, probe_id};
use rc_node::{
    ProcessChannel, ProcessEnvironment, ProcessExecutionMode, ProcessLifetime, ProcessPrincipal,
    ProcessStartRequest, StreamKind,
};
use std::time::{Duration, Instant};

pub(super) fn check(runtime: ComponentExecutionRuntime) -> anyhow::Result<()> {
    let execution = runtime
        .start(ProcessStartRequest {
            execution_id: probe_id("binary-stream-check"),
            mode: ProcessExecutionMode::Argv {
                program: std::env::current_exe()?.to_string_lossy().into_owned(),
                args: vec!["stream-fixture".into()],
            },
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
    let expected: Vec<u8> = (0..256).cycle().take(256 * 1024).map(|n| n as u8).collect();
    let mut offset = 0;
    let mut cursor = 0;
    let mut stdout = Vec::new();
    let mut stderr = Vec::new();
    let mut closed = false;
    let deadline = Instant::now() + Duration::from_secs(15);
    loop {
        anyhow::ensure!(Instant::now() < deadline, "binary stream probe timed out");
        let read = execution.read(cursor, 4096).map_err(anyhow::Error::msg)?;
        cursor = read.next_cursor;
        for chunk in read.chunks {
            match chunk.stream {
                StreamKind::Stdout => stdout.extend(chunk.bytes),
                StreamKind::Stderr => stderr.extend(chunk.bytes),
            }
        }
        // Wait for stderr while stdout is idle, then exercise simultaneous
        // input/output beyond both the OS pipe and host adapter capacities.
        if stderr == b"ready" && offset < expected.len() {
            offset += execution
                .input(&expected[offset..])
                .map_err(anyhow::Error::msg)? as usize;
        }
        if offset == expected.len() && !closed {
            execution.close_input().map_err(anyhow::Error::msg)?;
            closed = true;
        }
        if read.status == "exited" {
            anyhow::ensure!(read.exit_code == Some(0), "binary stream child failed");
            anyhow::ensure!(
                stdout == expected && stderr == b"ready",
                "binary stream bytes were lost or merged"
            );
            return Ok(());
        }
        std::thread::sleep(Duration::from_millis(1));
    }
}
