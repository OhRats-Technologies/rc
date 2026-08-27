use super::{ApiError, clean_name, public_signup_configured};
use crate::{AppState, RegistrationKind, active_user_count, start_registration};
use axum::{Json, extract::State};
use serde::Deserialize;
use uuid::Uuid;

#[derive(Deserialize)]
pub(super) struct SignupInput {
    name: String,
    turnstile: String,
}

pub(super) async fn options(
    State(state): State<AppState>,
    Json(input): Json<SignupInput>,
) -> Result<Json<serde_json::Value>, ApiError> {
    if !public_signup_configured(&state) || active_user_count(&state)? == 0 {
        return Err(ApiError::not_found("public signup is unavailable"));
    }
    verify_turnstile(&state, &input.turnstile).await?;
    let name = clean_name(&input.name)?;
    let user_id = Uuid::new_v4().to_string();
    let (ceremony_id, options) =
        start_registration(&state, RegistrationKind::Register, &user_id, &name, None)?;
    Ok(Json(
        serde_json::json!({ "ceremonyId": ceremony_id, "options": options.public_key }),
    ))
}

async fn verify_turnstile(state: &AppState, token: &str) -> Result<(), ApiError> {
    if token.trim().is_empty() {
        return Err(ApiError::bad_request(
            "Complete the human verification first.",
        ));
    }
    let secret = state
        .config
        .turnstile_secret_key
        .as_deref()
        .ok_or_else(|| ApiError::not_found("public signup is unavailable"))?;
    let body = url::form_urlencoded::Serializer::new(String::new())
        .append_pair("secret", secret)
        .append_pair("response", token)
        .finish();
    let response: serde_json::Value = reqwest::Client::new()
        .post("https://challenges.cloudflare.com/turnstile/v0/siteverify")
        .header("content-type", "application/x-www-form-urlencoded")
        .body(body)
        .send()
        .await
        .map_err(|_| ApiError::bad_gateway("human verification unavailable"))?
        .json()
        .await
        .map_err(|_| ApiError::bad_gateway("human verification unavailable"))?;
    if response.get("success").and_then(|value| value.as_bool()) != Some(true) {
        return Err(ApiError::unauthorized("human verification failed"));
    }
    Ok(())
}
