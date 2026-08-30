use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct IceServer {
    pub urls: Vec<String>,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub username: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub credential: String,
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum ControlIceMode {
    Host,
    #[default]
    Stun,
    Relay,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ControlIceAttempt {
    pub mode: ControlIceMode,
    pub route: ControlRouteClass,
    pub gather_timeout_ms: u32,
    pub connect_timeout_ms: u32,
    pub retry_delay_ms: u32,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "kebab-case")]
pub enum ControlRouteClass {
    DirectHost,
    DirectStun,
    TurnRelay,
    Unknown,
}

pub fn control_attempts_payload(attempts: &[ControlIceAttempt]) -> String {
    attempts
        .iter()
        .map(|attempt| {
            format!(
                "{}:{}:{}:{}:{}",
                ice_name(attempt.mode),
                route_name(attempt.route),
                attempt.gather_timeout_ms,
                attempt.connect_timeout_ms,
                attempt.retry_delay_ms
            )
        })
        .collect::<Vec<_>>()
        .join(",")
}

fn ice_name(mode: ControlIceMode) -> &'static str {
    match mode {
        ControlIceMode::Host => "host",
        ControlIceMode::Stun => "stun",
        ControlIceMode::Relay => "relay",
    }
}

fn route_name(route: ControlRouteClass) -> &'static str {
    match route {
        ControlRouteClass::DirectHost => "direct-host",
        ControlRouteClass::DirectStun => "direct-stun",
        ControlRouteClass::TurnRelay => "turn-relay",
        ControlRouteClass::Unknown => "unknown",
    }
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct NodeHello {
    pub version: String,
    pub hostname: String,
    pub platform: String,
    pub arch: String,
    #[serde(default)]
    pub capabilities: Vec<String>,
    pub transport_public_key: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub lock_hash: String,
    #[serde(default)]
    pub lock_generation: u64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct McpExecutionChunk {
    pub stream: String,
    pub cursor: u64,
    pub data: String,
}
