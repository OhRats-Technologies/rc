use rc_protocol::{ControlIceAttempt, ControlIceMode, IceServer};
use std::time::Duration;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TransportAnswerRequest {
    pub mode: ControlIceMode,
    pub ice_servers: Vec<IceServer>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TransportAnswerPlan {
    pub ice_servers: Vec<IceServer>,
    pub gather_timeout: Duration,
    pub connect_timeout: Duration,
}

pub trait TransportPolicy: Send + Sync {
    fn attempts(&self, ice_servers: Vec<IceServer>) -> Result<Vec<ControlIceAttempt>, String>;

    fn answer_plan(
        &self,
        transport: &str,
        request: TransportAnswerRequest,
    ) -> Result<TransportAnswerPlan, String>;
}
