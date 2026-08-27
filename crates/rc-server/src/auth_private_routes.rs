mod passkeys;

use crate::auth_public_routes::ApiError;
use crate::{
    AppState, approve_cli_authorization, browser_user, clear_session_cookie, consume_step_up,
    control_client_status, finish_control_authorization, finish_step_up, revoke_browser_session,
    revoke_cli_client, start_control_authorization, start_step_up, verify_client_request,
};
use axum::{
    Json, Router,
    extract::{Path, State},
    http::{HeaderMap, HeaderValue, Method, StatusCode},
    response::{IntoResponse, Response},
    routing::{delete, get, post},
};
use serde::Deserialize;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/auth/step-up/options", post(step_options))
        .route("/api/v1/auth/step-up/verify", post(step_verify))
        .route("/api/v1/control/authorize/options", post(control_options))
        .route("/api/v1/control/authorize/verify", post(control_verify))
        .route("/api/v1/control/clients/{id}", get(control_status))
        .route("/api/v1/auth/cli/approve", post(cli_approve))
        .route("/api/v1/auth/cli/session", delete(cli_logout))
        .route("/api/v1/auth/logout", post(browser_logout))
        .route("/api/v1/passkeys", get(passkeys::list))
        .route("/api/v1/passkeys/options", post(passkeys::options))
        .route("/api/v1/passkeys/verify", post(passkeys::verify))
        .route("/api/v1/passkeys/{id}", delete(passkeys::remove))
}

async fn step_options(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let user = require_browser(&state, &headers)?;
    let start = start_step_up(&state, &user)?;
    Ok(Json(
        serde_json::json!({ "authorizationId": start.authorization_id, "options": start.options.public_key }),
    ))
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct StepVerify {
    authorization_id: String,
    response: serde_json::Value,
}
async fn step_verify(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(input): Json<StepVerify>,
) -> Result<Json<crate::StepUpResult>, ApiError> {
    let user = require_browser(&state, &headers)?;
    Ok(Json(
        finish_step_up(&state, &user, &input.authorization_id, input.response).map_err(
            |error| {
                tracing::warn!(%error, "passkey step-up verification failed");
                ApiError::unauthorized("passkey verification failed")
            },
        )?,
    ))
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct ControlStart {
    client_id: String,
    signing_public_key: String,
    lifetime: Option<String>,
}
async fn control_options(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(input): Json<ControlStart>,
) -> Result<Json<serde_json::Value>, ApiError> {
    let user = require_browser(&state, &headers)?;
    let start = start_control_authorization(
        &state,
        &user,
        &input.client_id,
        &input.signing_public_key,
        input.lifetime.as_deref(),
        "browser",
    )
    .map_err(ApiError::bad_request_owned)?;
    Ok(Json(
        serde_json::json!({ "authorizationId": start.authorization_id, "grant": start.grant, "options": start.options.public_key }),
    ))
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct ControlVerify {
    authorization_id: String,
    response: serde_json::Value,
}
async fn control_verify(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(input): Json<ControlVerify>,
) -> Result<(StatusCode, Json<serde_json::Value>), ApiError> {
    let user = require_browser(&state, &headers)?;
    let (client_id, expires_at) =
        finish_control_authorization(&state, &user, &input.authorization_id, input.response)
            .map_err(|_| ApiError::unauthorized("passkey verification failed"))?;
    Ok((
        StatusCode::CREATED,
        Json(serde_json::json!({ "clientId": client_id, "expiresAt": expires_at })),
    ))
}

async fn control_status(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
) -> Result<Json<crate::ControlAuthorizationStatus>, ApiError> {
    let user = require_browser(&state, &headers)?;
    Ok(Json(control_client_status(&state, &user.id, &id)?))
}

#[derive(Deserialize)]
struct CliApprove {
    code: String,
}
async fn cli_approve(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(input): Json<CliApprove>,
) -> Result<Json<serde_json::Value>, ApiError> {
    let user = require_browser(&state, &headers)?;
    consume_step_up(&state, &headers, &user.id)
        .map_err(|_| ApiError::unauthorized("fresh passkey verification required"))?;
    approve_cli_authorization(&state, &user, &input.code)
        .map_err(|_| ApiError::gone("CLI authorization expired"))?;
    Ok(Json(serde_json::json!({"ok":true})))
}

async fn cli_logout(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let identity = verify_client_request(
        &state,
        &headers,
        &Method::DELETE,
        "/api/v1/auth/cli/session",
        &[],
    )
    .map_err(|_| ApiError::unauthorized("authentication required"))?;
    if identity.kind != "cli" {
        return Err(ApiError::bad_request("CLI session required"));
    }
    revoke_cli_client(&state, &identity.id, &identity.user_id)?;
    Ok(Json(serde_json::json!({"ok":true})))
}

async fn browser_logout(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Response, ApiError> {
    revoke_browser_session(&state, &headers)
        .map_err(|_| ApiError::unauthorized("authentication required"))?;
    let mut response = Json(serde_json::json!({"ok":true})).into_response();
    let cookie = clear_session_cookie(&state);
    let value = HeaderValue::from_str(&cookie).map_err(|_| {
        ApiError(
            StatusCode::INTERNAL_SERVER_ERROR,
            "failed to clear browser session".into(),
        )
    })?;
    response.headers_mut().insert("set-cookie", value);
    Ok(response)
}

pub(super) fn require_browser(
    state: &AppState,
    headers: &HeaderMap,
) -> Result<crate::UserIdentity, ApiError> {
    browser_user(state, headers)
        .map_err(|_| ApiError::unauthorized("authentication required"))?
        .ok_or_else(|| ApiError::unauthorized("browser session required"))
}
