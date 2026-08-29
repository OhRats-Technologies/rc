use rc_node::{
    ProcessAccessRequest, ProcessPolicy, ProcessResizeRequest, ProcessSignalRequest,
    ProcessStartPlan, ProcessStartRequest, ProcessTerminalSize, TransportAnswerPlan,
    TransportAnswerRequest, TransportPolicy,
};
use rc_protocol::{ControlIceAttempt, ControlIceMode, ControlRouteClass, IceServer};
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

    fn authorize_access(&self, _request: ProcessAccessRequest) -> Result<(), String> {
        Ok(())
    }

    fn normalize_resize(
        &self,
        request: ProcessResizeRequest,
    ) -> Result<ProcessTerminalSize, String> {
        Ok(ProcessTerminalSize {
            cols: request.cols,
            rows: request.rows,
        })
    }

    fn normalize_signal(&self, request: ProcessSignalRequest) -> Result<String, String> {
        Ok(request.signal)
    }
}

struct Transport;

impl TransportPolicy for Transport {
    fn attempts(&self, _ice_servers: Vec<IceServer>) -> Result<Vec<ControlIceAttempt>, String> {
        Ok(vec![ControlIceAttempt {
            mode: ControlIceMode::Host,
            route: ControlRouteClass::DirectHost,
            gather_timeout_ms: 2_000,
            connect_timeout_ms: 6_000,
            retry_delay_ms: 0,
        }])
    }

    fn answer_plan(
        &self,
        _transport: &str,
        request: TransportAnswerRequest,
    ) -> Result<TransportAnswerPlan, String> {
        Ok(TransportAnswerPlan {
            ice_servers: request.ice_servers,
            gather_timeout: Duration::from_secs(2),
            connect_timeout: Duration::from_secs(6),
        })
    }
}
