use crate::{AppState, AuthPrincipal, ControlSignalError, authenticate, control_proof};
use axum::{
    Json, Router,
    body::Bytes,
    extract::{Path, State},
    http::{HeaderMap, Method, StatusCode},
    response::{IntoResponse, Response},
    routing::{delete, post},
};
use serde::{Deserialize, Serialize};

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/control/challenge", post(challenge))
        .route("/api/v1/control/open", post(open))
        .route("/api/v1/control/{session_id}/webrtc", post(webrtc))
        .route("/api/v1/control/{session_id}", delete(close))
}

#[derive(Debug)]
struct ControlHttpError(StatusCode, String);

impl IntoResponse for ControlHttpError {
    fn into_response(self) -> Response {
        (self.0, Json(serde_json::json!({ "error": self.1 }))).into_response()
    }
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct ChallengeRequest {
    device_id: String,
}

#[derive(Serialize)]
struct ChallengeResponse {
    challenge: String,
}

async fn challenge(
    State(state): State<AppState>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<Json<ChallengeResponse>, ControlHttpError> {
    let path = "/api/v1/control/challenge";
    let principal = principal(&state, &headers, &Method::POST, path, &body)?;
    let input: ChallengeRequest = parse(&body)?;
    validate_id(&input.device_id, "device")?;
    require_control_principal(&state, &principal, &input.device_id)?;
    let challenge = state
        .control
        .challenge(&principal.user.id, &input.device_id)
        .await
        .map_err(signal_error)?;
    Ok(Json(ChallengeResponse { challenge }))
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct OpenRequest {
    device_id: String,
    challenge: String,
    client_id: String,
    public_key: String,
    signature: String,
}

async fn open(
    State(state): State<AppState>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<Json<crate::ControlReady>, ControlHttpError> {
    let path = "/api/v1/control/open";
    let principal = principal(&state, &headers, &Method::POST, path, &body)?;
    let input: OpenRequest = parse(&body)?;
    validate_id(&input.device_id, "device")?;
    validate_id(&input.client_id, "client")?;
    if input.challenge.is_empty()
        || input.challenge.len() > 512
        || input.public_key.is_empty()
        || input.public_key.len() > 512
        || input.signature.is_empty()
        || input.signature.len() > 512
    {
        return Err(bad_request("invalid control request"));
    }
    require_control_principal(&state, &principal, &input.device_id)?;
    let (client_id, proof) = if let Some(identity) = &principal.client {
        if input.client_id != identity.id {
            return Err(forbidden("control client identity mismatch"));
        }
        let proof = if identity.kind == "api" {
            None
        } else {
            control_proof(&state, &principal.user.id, &identity.id)
                .map_err(|_| internal())?
                .ok_or_else(|| unauthorized("control client authorization expired"))?
                .into()
        };
        (identity.id.clone(), proof)
    } else {
        let proof = control_proof(&state, &principal.user.id, &input.client_id)
            .map_err(|_| internal())?
            .ok_or_else(|| unauthorized("control client authorization expired"))?;
        (input.client_id.clone(), Some(proof))
    };
    let ready = state
        .control
        .open(
            &principal.user.id,
            &input.device_id,
            &client_id,
            &input.challenge,
            &input.public_key,
            &input.signature,
            proof.as_ref(),
        )
        .await
        .map_err(signal_error)?;
    Ok(Json(ready))
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct WebRtcRequest {
    device_id: String,
    sdp: String,
    #[serde(default)]
    relay: bool,
}

#[derive(Serialize)]
struct WebRtcResponse {
    sdp: String,
}

async fn webrtc(
    State(state): State<AppState>,
    Path(session_id): Path<String>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<Json<WebRtcResponse>, ControlHttpError> {
    validate_id(&session_id, "session")?;
    let path = format!("/api/v1/control/{session_id}/webrtc");
    let principal = principal(&state, &headers, &Method::POST, &path, &body)?;
    let input: WebRtcRequest = parse(&body)?;
    validate_id(&input.device_id, "device")?;
    if input.sdp.is_empty() || input.sdp.len() > 131_072 {
        return Err(bad_request("invalid WebRTC offer"));
    }
    require_control_principal(&state, &principal, &input.device_id)?;
    let sdp = state
        .control
        .webrtc(
            &principal.user.id,
            principal.client.as_ref().map(|client| client.id.as_str()),
            &session_id,
            &input.device_id,
            &input.sdp,
            input.relay,
        )
        .await
        .map_err(signal_error)?;
    Ok(Json(WebRtcResponse { sdp }))
}

async fn close(
    State(state): State<AppState>,
    Path(session_id): Path<String>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ControlHttpError> {
    validate_id(&session_id, "session")?;
    let path = format!("/api/v1/control/{session_id}");
    let principal = principal(&state, &headers, &Method::DELETE, &path, &[])?;
    state
        .control
        .close(
            &principal.user.id,
            principal.client.as_ref().map(|client| client.id.as_str()),
            &session_id,
        )
        .await;
    Ok(Json(serde_json::json!({ "ok": true })))
}

fn require_control_principal(
    state: &AppState,
    principal: &AuthPrincipal,
    device_id: &str,
) -> Result<(), ControlHttpError> {
    let role = state
        .db
        .device_role(&principal.user.id, device_id)
        .map_err(|_| internal())?
        .ok_or_else(|| forbidden("operator required"))?;
    if !matches!(role.as_str(), "owner" | "operator") {
        return Err(forbidden("operator required"));
    }
    if principal.client.as_ref().is_some_and(|identity| {
        identity.kind == "api"
            && !identity.has_scope("execute")
            && !identity.has_scope("manage-devices")
    }) {
        return Err(forbidden(
            "API key requires execute or manage-devices scope",
        ));
    }
    if principal
        .client
        .as_ref()
        .is_some_and(|identity| !matches!(identity.kind.as_str(), "api" | "cli" | "browser"))
    {
        return Err(forbidden("unsupported control client"));
    }
    Ok(())
}

fn parse<T: serde::de::DeserializeOwned>(body: &[u8]) -> Result<T, ControlHttpError> {
    serde_json::from_slice(body).map_err(|_| bad_request("invalid request"))
}

fn validate_id(value: &str, label: &'static str) -> Result<(), ControlHttpError> {
    if value.is_empty() || value.len() > 100 {
        return Err(bad_request(match label {
            "device" => "invalid device id",
            "client" => "invalid client id",
            "session" => "invalid control session",
            _ => "invalid identifier",
        }));
    }
    Ok(())
}

fn signal_error(error: ControlSignalError) -> ControlHttpError {
    match error {
        ControlSignalError::Offline | ControlSignalError::Disconnected => {
            ControlHttpError(StatusCode::CONFLICT, error.to_string())
        }
        ControlSignalError::Unavailable => {
            ControlHttpError(StatusCode::NOT_FOUND, error.to_string())
        }
        ControlSignalError::Timeout => {
            ControlHttpError(StatusCode::GATEWAY_TIMEOUT, error.to_string())
        }
        ControlSignalError::Turn | ControlSignalError::Protocol => {
            ControlHttpError(StatusCode::BAD_GATEWAY, error.to_string())
        }
        ControlSignalError::Rejected(_) => {
            ControlHttpError(StatusCode::CONFLICT, error.to_string())
        }
    }
}

fn bad_request(message: &'static str) -> ControlHttpError {
    ControlHttpError(StatusCode::BAD_REQUEST, message.into())
}

fn forbidden(message: &'static str) -> ControlHttpError {
    ControlHttpError(StatusCode::FORBIDDEN, message.into())
}

fn unauthorized(message: &'static str) -> ControlHttpError {
    ControlHttpError(StatusCode::UNAUTHORIZED, message.into())
}

fn principal(
    state: &AppState,
    headers: &HeaderMap,
    method: &Method,
    path: &str,
    body: &[u8],
) -> Result<AuthPrincipal, ControlHttpError> {
    authenticate(state, headers, method, path, body)
        .map_err(|error| ControlHttpError(error.status(), error.to_string()))?
        .ok_or_else(|| unauthorized("authentication required"))
}

fn internal() -> ControlHttpError {
    ControlHttpError(StatusCode::INTERNAL_SERVER_ERROR, "database error".into())
}
