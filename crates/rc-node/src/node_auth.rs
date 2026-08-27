use crate::NodeState;
use rc_api_client::random_url_bytes;
use rc_crypto::sign_node_http;

#[derive(Debug, Clone)]
pub struct NodeHttpAuth {
    pub device_id: String,
    pub timestamp: String,
    pub nonce: String,
    pub signature: String,
}

impl NodeHttpAuth {
    pub fn headers(&self) -> [(&'static str, &str); 4] {
        [
            ("x-rc-device", &self.device_id),
            ("x-rc-timestamp", &self.timestamp),
            ("x-rc-nonce", &self.nonce),
            ("x-rc-signature", &self.signature),
        ]
    }
}

pub fn sign_node_request(
    state: &NodeState,
    method: &str,
    path: &str,
    body: &[u8],
) -> Result<NodeHttpAuth, rc_crypto::CryptoError> {
    let timestamp = unix_seconds().to_string();
    let nonce = random_url_bytes(18);
    let signature = sign_node_http(
        &state.identity_seed,
        &state.device_id,
        &timestamp,
        &nonce,
        method,
        path,
        body,
    )?;
    Ok(NodeHttpAuth {
        device_id: state.device_id.clone(),
        timestamp,
        nonce,
        signature,
    })
}

fn unix_seconds() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}
