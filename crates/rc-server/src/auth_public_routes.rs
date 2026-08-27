mod cli;
mod error;
mod signup;

pub(crate) use error::ApiError;

use crate::{
    AppState, RegistrationKind, WEB_DEFAULT_LIFETIME, active_user_count, auth_lifetime,
    create_browser_session, finish_login, finish_registration, hash, insert_passkey, now_ms,
    session_cookie, start_login, start_registration,
};
use axum::{
    Json, Router,
    extract::State,
    http::{HeaderMap, HeaderValue, StatusCode},
    response::{IntoResponse, Response},
    routing::{get, post},
};
use rusqlite::OptionalExtension;
use serde::Deserialize;
use uuid::Uuid;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route(
            "/api/v1/health",
            get(|| async { Json(serde_json::json!({"ok": true})) }),
        )
        .route("/api/v1/status", get(status))
        .route("/api/v1/auth/setup/options", post(setup_options))
        .route("/api/v1/auth/setup/verify", post(setup_verify))
        .route("/api/v1/auth/login/options", post(login_options))
        .route("/api/v1/auth/login/verify", post(login_verify))
        .route("/api/v1/auth/register/options", post(register_options))
        .route("/api/v1/auth/register/verify", post(register_verify))
        .route("/api/v1/auth/signup/options", post(signup::options))
        .route("/api/v1/auth/cli/start", post(cli::start))
        .route("/api/v1/auth/cli/poll", post(cli::poll))
}

async fn status(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let users = active_user_count(&state)?;
    let setup_required = users == 0;
    Ok(Json(serde_json::json!({
        "setupRequired": setup_required,
        "setupAuthorized": setup_required && setup_authorized(&state, &headers),
        "publicSignup": users > 0 && public_signup_configured(&state),
        "version": env!("CARGO_PKG_VERSION"),
    })))
}

#[derive(Deserialize)]
struct NameInput {
    name: String,
}

async fn setup_options(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(input): Json<NameInput>,
) -> Result<Json<serde_json::Value>, ApiError> {
    if active_user_count(&state)? > 0 {
        return Err(ApiError::conflict("setup already completed"));
    }
    if !setup_authorized(&state, &headers) {
        return Err(ApiError::forbidden("Open the RC setup link first."));
    }
    let name = clean_name(&input.name)?;
    let user_id = Uuid::new_v4().to_string();
    let (ceremony_id, options) =
        start_registration(&state, RegistrationKind::Setup, &user_id, &name, None)?;
    Ok(Json(
        serde_json::json!({ "ceremonyId": ceremony_id, "options": options.public_key }),
    ))
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct VerifyInput {
    ceremony_id: String,
    response: serde_json::Value,
}

async fn setup_verify(
    State(state): State<AppState>,
    Json(input): Json<VerifyInput>,
) -> Result<Response, ApiError> {
    let finished = finish_registration(
        &state,
        RegistrationKind::Setup,
        &input.ceremony_id,
        input.response,
    )
    .map_err(|_| ApiError::unauthorized("passkey verification failed"))?;
    let user_id = finished.user_id.clone();
    state.db.with_connection_mut(|db| {
        let tx = db.transaction()?;
        let count: i64 = tx.query_row(
            "SELECT count(*) FROM users WHERE id<>?",
            [crate::DELETED_USER_ID],
            |row| row.get(0),
        )?;
        if count != 0 { return Err(rusqlite::Error::InvalidQuery); }
        tx.execute("INSERT INTO users(id,name,created_at) VALUES(?,?,?)", rusqlite::params![user_id, finished.name, now_ms()])?;
        insert_passkey(&tx, &user_id, &finished.passkey)?;
        let workspace_id = Uuid::new_v4().to_string();
        tx.execute("INSERT INTO workspaces(id,name,created_by,created_at) VALUES(?,?,?,?)", rusqlite::params![workspace_id, "Personal", user_id, now_ms()])?;
        tx.execute("INSERT INTO workspace_members(workspace_id,user_id,role,joined_at) VALUES(?,?,'owner',?)", rusqlite::params![workspace_id, user_id, now_ms()])?;
        tx.commit()
    }).map_err(|_| ApiError::conflict("setup already completed"))?;
    login_response(&state, &user_id, None, StatusCode::CREATED)
}

async fn login_options(State(state): State<AppState>) -> Result<Json<serde_json::Value>, ApiError> {
    let (ceremony_id, options) =
        start_login(&state).map_err(|_| ApiError::conflict("no passkeys registered"))?;
    Ok(Json(
        serde_json::json!({ "ceremonyId": ceremony_id, "options": options.public_key }),
    ))
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct LoginVerify {
    ceremony_id: String,
    response: serde_json::Value,
    lifetime: Option<String>,
}

async fn login_verify(
    State(state): State<AppState>,
    Json(input): Json<LoginVerify>,
) -> Result<Response, ApiError> {
    let user = finish_login(&state, &input.ceremony_id, input.response)
        .map_err(|_| ApiError::unauthorized("passkey verification failed"))?;
    login_response(&state, &user.id, input.lifetime.as_deref(), StatusCode::OK)
}

#[derive(Deserialize)]
struct RegisterInput {
    invite: String,
    name: String,
}

async fn register_options(
    State(state): State<AppState>,
    Json(input): Json<RegisterInput>,
) -> Result<Json<serde_json::Value>, ApiError> {
    let invite_id = valid_invite(&state, &input.invite)?
        .ok_or_else(|| ApiError::unauthorized("invalid or expired invite"))?;
    let name = clean_name(&input.name)?;
    let user_id = Uuid::new_v4().to_string();
    let (ceremony_id, options) = start_registration(
        &state,
        RegistrationKind::Register,
        &user_id,
        &name,
        Some(&invite_id),
    )?;
    Ok(Json(
        serde_json::json!({ "ceremonyId": ceremony_id, "options": options.public_key }),
    ))
}

async fn register_verify(
    State(state): State<AppState>,
    Json(input): Json<VerifyInput>,
) -> Result<Response, ApiError> {
    let finished = finish_registration(
        &state,
        RegistrationKind::Register,
        &input.ceremony_id,
        input.response,
    )
    .map_err(|_| ApiError::unauthorized("passkey verification failed"))?;
    let user_id = finished.user_id.clone();
    state.db.with_connection_mut(|db| {
        let tx = db.transaction()?;
        tx.execute("INSERT INTO users(id,name,created_at) VALUES(?,?,?)", rusqlite::params![user_id, finished.name, now_ms()])?;
        insert_passkey(&tx, &user_id, &finished.passkey)?;
        let personal = Uuid::new_v4().to_string();
        tx.execute("INSERT INTO workspaces(id,name,created_by,created_at) VALUES(?,?,?,?)", rusqlite::params![personal, "Personal", user_id, now_ms()])?;
        tx.execute("INSERT INTO workspace_members(workspace_id,user_id,role,joined_at) VALUES(?,?,'owner',?)", rusqlite::params![personal, user_id, now_ms()])?;
        if let Some(invite_id) = finished.invite_id {
            let invite: Option<(String,String)> = tx.query_row("SELECT workspace_id,role FROM workspace_invites WHERE id=? AND used_at IS NULL AND expires_at>?", rusqlite::params![invite_id, now_ms()], |row| Ok((row.get(0)?,row.get(1)?))).optional()?;
            let Some((workspace, role)) = invite else { return Err(rusqlite::Error::InvalidQuery); };
            if tx.execute("UPDATE workspace_invites SET used_at=? WHERE id=? AND used_at IS NULL", rusqlite::params![now_ms(), invite_id])? != 1 { return Err(rusqlite::Error::InvalidQuery); }
            tx.execute("INSERT INTO workspace_members(workspace_id,user_id,role,joined_at) VALUES(?,?,?,?)", rusqlite::params![workspace,user_id,role,now_ms()])?;
        }
        tx.commit()
    }).map_err(|_| ApiError::unauthorized("invalid or expired invite"))?;
    login_response(&state, &user_id, None, StatusCode::CREATED)
}

fn login_response(
    state: &AppState,
    user_id: &str,
    lifetime: Option<&str>,
    status: StatusCode,
) -> Result<Response, ApiError> {
    let lifetime = auth_lifetime(lifetime, WEB_DEFAULT_LIFETIME, false, now_ms())
        .map_err(ApiError::bad_request)?;
    let token = create_browser_session(state, user_id, lifetime.expires_at)?;
    let mut response = (
        status,
        Json(serde_json::json!({ "ok": true, "expiresAt": lifetime.expires_at })),
    )
        .into_response();
    let cookie = session_cookie(state, &token, lifetime.max_age.unwrap_or(0));
    let value = HeaderValue::from_str(&cookie).map_err(|_| {
        ApiError(
            StatusCode::INTERNAL_SERVER_ERROR,
            "failed to create browser session".into(),
        )
    })?;
    response.headers_mut().insert("set-cookie", value);
    Ok(response)
}

fn clean_name(value: &str) -> Result<String, ApiError> {
    let value = value.trim().chars().take(120).collect::<String>();
    if value.is_empty() {
        Err(ApiError::bad_request("name required"))
    } else {
        Ok(value)
    }
}

fn valid_invite(state: &AppState, token: &str) -> Result<Option<String>, ApiError> {
    use rusqlite::OptionalExtension;
    Ok(state.db.with_connection(|db| db.query_row("SELECT id FROM workspace_invites WHERE token_hash=? AND used_at IS NULL AND expires_at>?", rusqlite::params![hash(token.trim()), now_ms()], |row| row.get(0)).optional())?)
}

fn setup_authorized(state: &AppState, headers: &HeaderMap) -> bool {
    let Some(expected) = state.config.setup_token.as_deref() else {
        return true;
    };
    cookie(headers, "rc_setup").is_some_and(|token| hash(&token) == hash(expected))
}
fn cookie(headers: &HeaderMap, name: &str) -> Option<String> {
    headers
        .get("cookie")?
        .to_str()
        .ok()?
        .split(';')
        .filter_map(|v| v.trim().split_once('='))
        .find(|(key, _)| *key == name)
        .map(|(_, v)| v.to_owned())
}
pub(super) fn public_signup_configured(state: &AppState) -> bool {
    state.config.public_signup
        && state.config.turnstile_site_key.is_some()
        && state.config.turnstile_secret_key.is_some()
}
