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
            mode: request.mode,
            cwd: request.cwd,
            environment: request.environment,
            terminal: request.terminal,
            scrollback_bytes: 4 << 20,
            stdin_chunk_bytes: 1 << 20,
            authorization_timeout_ms: 15_000,
            terminate_grace_ms: 350,
            reattach_grace_ms: 60_000,
            max_runtime_ms: request.max_runtime_ms,
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
    fn authorize_signal(
        &self,
        value: ProcessSignalRequest,
    ) -> Result<rc_node::ProcessSignal, String> {
        Ok(value.signal)
    }
}

struct Transport;

impl TransportPolicy for Transport {
    fn attempts(&self, _: Vec<IceServer>) -> Result<Vec<ControlIceAttempt>, String> {
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
