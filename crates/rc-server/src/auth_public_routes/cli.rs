use super::ApiError;
use crate::{AppState, poll_cli_authorization, start_cli_authorization};
use axum::{Json, extract::State};
use serde::Deserialize;

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub(super) struct CliStart {
    client_id: String,
    signing_public_key: String,
    lifetime: Option<String>,
}

pub(super) async fn start(
    State(state): State<AppState>,
    Json(input): Json<CliStart>,
) -> Result<Json<crate::CliAuthorizationStart>, ApiError> {
    Ok(Json(
        start_cli_authorization(
            &state,
            &input.client_id,
            &input.signing_public_key,
            input.lifetime.as_deref(),
        )
        .map_err(ApiError::bad_request_owned)?,
    ))
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub(super) struct CliPoll {
    request_id: String,
    device_code: String,
}

pub(super) async fn poll(
    State(state): State<AppState>,
    Json(input): Json<CliPoll>,
) -> Result<Json<crate::CliPollResult>, ApiError> {
    Ok(Json(
        poll_cli_authorization(&state, &input.request_id, &input.device_code)
            .map_err(|_| ApiError::gone("CLI authorization expired"))?,
    ))
}
