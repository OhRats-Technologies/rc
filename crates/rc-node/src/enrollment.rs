use crate::{NODE_CAPABILITIES, NodeState, sign_node_request};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use ed25519_dalek::SigningKey;
use rand::RngCore;
use rc_api_client::{ApiError, public_post};
use reqwest::Method;
use serde::{Deserialize, Serialize};
use x25519_dalek::{PublicKey, StaticSecret};

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RemoteNodeStatus {
    pub name: String,
    pub online: bool,
    pub version: String,
}

#[derive(Debug, thiserror::Error)]
pub enum EnrollmentError {
    #[error(transparent)]
    Api(#[from] ApiError),
    #[error(transparent)]
    Transport(#[from] reqwest::Error),
    #[error(transparent)]
    Crypto(#[from] rc_crypto::CryptoError),
    #[error("node removed from RC")]
    Removed,
    #[error("{0}")]
    Server(String),
}

pub async fn enroll(
    server: &str,
    token: &str,
    display_name: &str,
    version: &str,
) -> Result<NodeState, EnrollmentError> {
    #[derive(Serialize)]
    #[serde(rename_all = "camelCase")]
    struct Request<'a> {
        token: &'a str,
        name: String,
        hostname: String,
        platform: &'static str,
        arch: &'static str,
        identity_public_key: String,
        transport_public_key: String,
        version: &'a str,
        capabilities: Vec<&'static str>,
    }
    #[derive(Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct Response {
        device_id: String,
    }

    let identity = SigningKey::generate(&mut rand::rngs::OsRng);
    let mut transport_bytes = [0_u8; 32];
    rand::rngs::OsRng.fill_bytes(&mut transport_bytes);
    let transport = StaticSecret::from(transport_bytes);
    let transport_public = PublicKey::from(&transport);
    let hostname = hostname();
    let name = if display_name.trim().is_empty() {
        hostname.clone()
    } else {
        display_name.to_owned()
    };
    let response: Response = public_post(
        server,
        "/api/v1/node/enroll",
        &Request {
            token,
            name,
            hostname,
            platform: platform(),
            arch: arch(),
            identity_public_key: URL_SAFE_NO_PAD.encode(identity.verifying_key().as_bytes()),
            transport_public_key: URL_SAFE_NO_PAD.encode(transport_public.as_bytes()),
            version,
            capabilities: NODE_CAPABILITIES.to_vec(),
        },
    )
    .await?;
    Ok(NodeState {
        v: crate::STATE_VERSION,
        device_id: response.device_id,
        identity_seed: URL_SAFE_NO_PAD.encode(identity.to_bytes()),
        transport_secret: URL_SAFE_NO_PAD.encode(transport.to_bytes()),
    })
}

pub async fn fetch_status(
    server: &str,
    state: &NodeState,
) -> Result<RemoteNodeStatus, EnrollmentError> {
    let path = "/api/v1/node/self";
    let auth = sign_node_request(state, Method::GET.as_str(), path, &[])?;
    let mut request =
        reqwest::Client::new().get(format!("{}{}", server.trim_end_matches('/'), path));
    for (name, value) in auth.headers() {
        request = request.header(name, value);
    }
    let response = request.send().await?;
    if response.status().as_u16() == 410 {
        return Err(EnrollmentError::Removed);
    }
    if !response.status().is_success() {
        return Err(EnrollmentError::Server(
            response.text().await.unwrap_or_default(),
        ));
    }
    Ok(response.json().await?)
}

pub async fn unregister(server: &str, state: &NodeState) -> Result<(), EnrollmentError> {
    let path = "/api/v1/node/self";
    let auth = sign_node_request(state, Method::DELETE.as_str(), path, &[])?;
    let mut request =
        reqwest::Client::new().delete(format!("{}{}", server.trim_end_matches('/'), path));
    for (name, value) in auth.headers() {
        request = request.header(name, value);
    }
    let response = request.send().await?;
    if response.status().is_success() || matches!(response.status().as_u16(), 404 | 410) {
        return Ok(());
    }
    Err(EnrollmentError::Server(
        response.text().await.unwrap_or_default(),
    ))
}

fn hostname() -> String {
    std::process::Command::new("hostname")
        .output()
        .ok()
        .and_then(|out| String::from_utf8(out.stdout).ok())
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty())
        .unwrap_or_else(|| "localhost".into())
}
fn platform() -> &'static str {
    match std::env::consts::OS {
        "macos" => "darwin",
        value => value,
    }
}
fn arch() -> &'static str {
    match std::env::consts::ARCH {
        "aarch64" => "arm64",
        "x86_64" => "amd64",
        value => value,
    }
}
