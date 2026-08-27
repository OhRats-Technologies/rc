use rc_protocol::IceServer;
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Deserialize)]
pub struct Device {
    pub id: String,
    pub name: String,
    #[serde(default, rename = "workspace_name")]
    pub workspace: String,
    #[serde(default)]
    pub version: String,
    #[serde(default)]
    pub capabilities: Vec<String>,
    #[serde(default)]
    pub online: bool,
    #[serde(default, rename = "identity_public_key")]
    pub identity_public_key: String,
    #[serde(default, rename = "transport_public_key")]
    pub transport_public_key: String,
}

impl Device {
    pub fn supports(&self, capability: &str) -> bool {
        self.capabilities.iter().any(|value| value == capability)
    }
}

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ControlReady {
    pub session_id: String,
    pub transport_public_key: String,
    pub ephemeral_public_key: String,
    pub signature: String,
    #[serde(default)]
    pub ice_servers: Vec<IceServer>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct ControlChallenge {
    pub challenge: String,
}

#[derive(Debug, Clone, Deserialize)]
pub struct WebRtcAnswer {
    pub sdp: String,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ControlOpen<'a> {
    pub device_id: &'a str,
    pub challenge: &'a str,
    pub client_id: &'a str,
    pub public_key: &'a str,
    pub signature: &'a str,
}
