use rc_node::{
    ProcessAccessRequest, ProcessPolicy, ProcessResizeRequest, ProcessSignalRequest,
    ProcessStartPlan, ProcessStartRequest, ProcessTerminalSize, TransportAnswerPlan,
    TransportAnswerRequest, TransportPolicy,
};
use rc_protocol::{ControlIceAttempt, ControlIceMode, IceServer};
use std::{sync::Arc, time::Duration};

pub fn pair() -> (Arc<dyn ProcessPolicy>, Arc<dyn TransportPolicy>) {
    (Arc::new(Process), Arc::new(Transport))
}

struct Process;

impl ProcessPolicy for Process {
    fn authorize_start(&self, request: ProcessStartRequest) -> Result<ProcessStartPlan, String> {
        Ok(ProcessStartPlan {
            command: request.command,
            cwd: request.cwd,
            terminal: request.terminal,
            scrollback_bytes: 4 << 20,
            stdin_chunk_bytes: 1 << 20,
        })
    }
    fn authorize_access(&self, _: ProcessAccessRequest) -> Result<(), String> {
        Ok(())
    }
    fn normalize_resize(&self, value: ProcessResizeRequest) -> Result<ProcessTerminalSize, String> {
        Ok(ProcessTerminalSize {
            cols: value.cols,
            rows: value.rows,
        })
    }
    fn normalize_signal(&self, value: ProcessSignalRequest) -> Result<String, String> {
        Ok(value.signal)
    }
}

struct Transport;

impl TransportPolicy for Transport {
    fn attempts(&self, _: Vec<IceServer>) -> Result<Vec<ControlIceAttempt>, String> {
        Ok(vec![ControlIceAttempt {
            mode: ControlIceMode::Host,
            gather_timeout_ms: 2_000,
            connect_timeout_ms: 6_000,
            retry_delay_ms: 0,
        }])
    }

    fn answer_plan(
        &self,
        _: &str,
        request: TransportAnswerRequest,
    ) -> Result<TransportAnswerPlan, String> {
        Ok(TransportAnswerPlan {
            ice_servers: request.ice_servers,
            gather_timeout: Duration::from_secs(2),
            connect_timeout: Duration::from_secs(6),
        })
    }
}
