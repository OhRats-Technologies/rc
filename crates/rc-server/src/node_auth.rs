use crate::{AppState, now_ms};
use axum::{
    http::{HeaderMap, Method, StatusCode},
    response::{IntoResponse, Response},
};
use rc_crypto::{node_http_payload, verify_node_http};
use sha2::{Digest, Sha256};

#[derive(Debug, Clone)]
pub struct NodeIdentity {
    pub id: String,
}

#[derive(Debug)]
pub struct HttpError(pub StatusCode, pub &'static str);

impl IntoResponse for HttpError {
    fn into_response(self) -> Response {
        (self.0, axum::Json(serde_json::json!({ "error": self.1 }))).into_response()
    }
}

pub fn verify_node_request(
    state: &AppState,
    headers: &HeaderMap,
    method: &Method,
    path: &str,
    body: &[u8],
) -> Result<NodeIdentity, HttpError> {
    let device = header(headers, "x-rc-device")?;
    let timestamp = header(headers, "x-rc-timestamp")?;
    let nonce = header(headers, "x-rc-nonce")?;
    let signature = header(headers, "x-rc-signature")?;
    let seconds = timestamp
        .parse::<i64>()
        .map_err(|_| HttpError(StatusCode::UNAUTHORIZED, "invalid Node authentication"))?;
    let now_seconds = now_ms() / 1000;
    if (now_seconds - seconds).abs() > 60 || nonce.len() < 16 || nonce.len() > 128 {
        return Err(HttpError(
            StatusCode::UNAUTHORIZED,
            "expired Node authentication",
        ));
    }

    let active = state.db.device_public_key(device).map_err(internal)?;
    let revoked = if active.is_none() {
        state.db.revoked_public_key(device).map_err(internal)?
    } else {
        None
    };
    let Some(public_key) = active.as_ref().or(revoked.as_ref()) else {
        return Err(HttpError(StatusCode::NOT_FOUND, "Node not found"));
    };
    let payload = node_http_payload(device, timestamp, nonce, method.as_str(), path, body);
    verify_node_http(public_key, signature, &payload)
        .map_err(|_| HttpError(StatusCode::UNAUTHORIZED, "invalid Node authentication"))?;
    if revoked.is_some() {
        return Err(HttpError(StatusCode::GONE, "Node removed"));
    }

    let hash = hex_lower(&Sha256::digest(nonce.as_bytes()));
    let fresh = state
        .db
        .remember_nonce(&format!("node:{device}"), &hash, now_ms() + 120_000)
        .map_err(internal)?;
    if !fresh {
        return Err(HttpError(StatusCode::CONFLICT, "replayed Node request"));
    }
    Ok(NodeIdentity {
        id: device.to_owned(),
    })
}

fn header<'a>(headers: &'a HeaderMap, name: &'static str) -> Result<&'a str, HttpError> {
    headers
        .get(name)
        .and_then(|value| value.to_str().ok())
        .filter(|value| !value.is_empty())
        .ok_or(HttpError(
            StatusCode::UNAUTHORIZED,
            "missing Node authentication",
        ))
}
fn internal(_: rusqlite::Error) -> HttpError {
    HttpError(StatusCode::INTERNAL_SERVER_ERROR, "database error")
}
fn hex_lower(bytes: &[u8]) -> String {
    bytes.iter().map(|byte| format!("{byte:02x}")).collect()
}
