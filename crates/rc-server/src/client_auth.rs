use crate::{AppState, now_ms};
use axum::{
    http::{HeaderMap, Method, StatusCode},
    response::{IntoResponse, Response},
};
use rc_crypto::{ApiRequest, verify_api};
use sha2::{Digest, Sha256};

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ClientIdentity {
    pub id: String,
    pub user_id: String,
    pub kind: String,
    pub scopes: Vec<String>,
    pub grant: String,
    pub credential_id: String,
    pub assertion: String,
}

impl ClientIdentity {
    pub fn has_scope(&self, scope: &str) -> bool {
        self.scopes.iter().any(|value| value == scope)
    }

    pub fn is_human_client(&self) -> bool {
        matches!(self.kind.as_str(), "browser" | "cli")
    }
}

#[derive(Debug)]
pub struct ClientHttpError(pub StatusCode, pub &'static str);

impl IntoResponse for ClientHttpError {
    fn into_response(self) -> Response {
        (self.0, axum::Json(serde_json::json!({ "error": self.1 }))).into_response()
    }
}

pub fn verify_client_request(
    state: &AppState,
    headers: &HeaderMap,
    method: &Method,
    request_uri: &str,
    body: &[u8],
) -> Result<ClientIdentity, ClientHttpError> {
    let key_id = header(headers, "x-rc-key-id")?;
    let timestamp = header(headers, "x-rc-timestamp")?;
    let nonce = header(headers, "x-rc-nonce")?;
    let signature = header(headers, "x-rc-signature")?;
    let seconds = timestamp
        .parse::<i64>()
        .map_err(|_| unauthorized("invalid client authentication"))?;
    if (now_ms() / 1000 - seconds).abs() > 60 || nonce.len() < 16 || nonce.len() > 128 {
        return Err(unauthorized("expired client authentication"));
    }

    let row = state
        .db
        .client_auth(key_id)
        .map_err(internal)?
        .ok_or_else(|| unauthorized("client authentication rejected"))?;
    verify_api(
        &row.public_key,
        signature,
        ApiRequest {
            key_id,
            timestamp,
            nonce,
            method: method.as_str(),
            request_uri,
            body,
        },
    )
    .map_err(|_| unauthorized("invalid client authentication"))?;

    let hash = hex_lower(&Sha256::digest(nonce.as_bytes()));
    if !state
        .db
        .remember_nonce(&format!("client:{key_id}"), &hash, now_ms() + 120_000)
        .map_err(internal)?
    {
        return Err(ClientHttpError(
            StatusCode::CONFLICT,
            "replayed client request",
        ));
    }
    state.db.touch_client(key_id).map_err(internal)?;
    let scopes = serde_json::from_str::<Vec<String>>(&row.scopes).unwrap_or_default();
    Ok(ClientIdentity {
        id: key_id.to_owned(),
        user_id: row.user_id,
        kind: row.kind,
        scopes,
        grant: row.grant.unwrap_or_default(),
        credential_id: row.credential_id.unwrap_or_default(),
        assertion: row.assertion.unwrap_or_default(),
    })
}

fn header<'a>(headers: &'a HeaderMap, name: &'static str) -> Result<&'a str, ClientHttpError> {
    headers
        .get(name)
        .and_then(|value| value.to_str().ok())
        .filter(|value| !value.is_empty())
        .ok_or_else(|| unauthorized("missing client authentication"))
}

fn unauthorized(message: &'static str) -> ClientHttpError {
    ClientHttpError(StatusCode::UNAUTHORIZED, message)
}

fn internal(_: rusqlite::Error) -> ClientHttpError {
    ClientHttpError(StatusCode::INTERNAL_SERVER_ERROR, "database error")
}

fn hex_lower(bytes: &[u8]) -> String {
    bytes.iter().map(|byte| format!("{byte:02x}")).collect()
}
