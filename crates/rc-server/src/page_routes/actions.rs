use crate::auth_public_routes::ApiError;
use crate::{AppState, browser_user, now_ms, workspace_role};
use axum::{
    Router,
    body::Bytes,
    extract::{Path, State},
    http::{HeaderMap, StatusCode},
    response::{IntoResponse, Response},
    routing::post,
};
use uuid::Uuid;

pub(super) fn routes() -> Router<AppState> {
    Router::new()
        .route("/workspaces", post(workspace_create))
        .route("/workspaces/{id}/rename", post(workspace_rename))
        .route("/workspaces/{id}/leave", post(workspace_leave))
        .route("/devices/{id}/rename", post(device_rename))
}

async fn workspace_create(
    State(state): State<AppState>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<Response, ApiError> {
    let user = user(&state, &headers)?;
    let values = form(&body);
    let name = clean(
        values.get("name").map(String::as_str).unwrap_or_default(),
        "workspace name required",
    )?;
    let count = state.db.with_connection(|db| {
        db.query_row(
            "SELECT count(*) FROM workspaces WHERE created_by=?",
            [&user.id],
            |row| row.get::<_, i64>(0),
        )
    })?;
    if count >= 10 {
        return Err(ApiError::conflict("workspace limit reached (10)"));
    }
    let id = Uuid::new_v4().to_string();
    state.db.with_connection_mut(|db| {
        let tx = db.transaction()?;
        tx.execute("INSERT INTO workspaces(id,name,created_by,created_at) VALUES(?,?,?,?)", rusqlite::params![id,name,user.id,now_ms()])?;
        tx.execute("INSERT INTO workspace_members(workspace_id,user_id,role,joined_at) VALUES(?,?,'owner',?)", rusqlite::params![id,user.id,now_ms()])?;
        tx.commit()
    })?;
    state.events.emit(
        &state.db,
        "workspace.created",
        Some(&id),
        Some(&user.id),
        None,
        serde_json::json!({"name":name}),
    )?;
    Ok(redirect(next(&values, "/devices")))
}

async fn workspace_rename(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<Response, ApiError> {
    let user = user(&state, &headers)?;
    owner_workspace(&state, &user.id, &id)?;
    let values = form(&body);
    let name = clean(
        values.get("name").map(String::as_str).unwrap_or_default(),
        "workspace name required",
    )?;
    if state.db.with_connection(|db| {
        db.execute(
            "UPDATE workspaces SET name=? WHERE id=?",
            rusqlite::params![name, id],
        )
    })? == 0
    {
        return Err(ApiError::not_found("workspace not found"));
    }
    state.events.emit(
        &state.db,
        "workspace.renamed",
        Some(&id),
        Some(&user.id),
        None,
        serde_json::json!({"name":name}),
    )?;
    Ok(redirect(next(&values, "/devices")))
}

async fn workspace_leave(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
) -> Result<Response, ApiError> {
    let user = user(&state, &headers)?;
    crate::workspace_routes::leave_workspace(&state, &user.id, &id)?;
    Ok(redirect("/devices"))
}

async fn device_rename(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<Response, ApiError> {
    let user = user(&state, &headers)?;
    if state.db.device_role(&user.id, &id)?.as_deref() != Some("owner") {
        return Err(ApiError::forbidden("owner required"));
    }
    let values = form(&body);
    let name = clean(
        values.get("name").map(String::as_str).unwrap_or_default(),
        "device name required",
    )?;
    if state.db.with_connection(|db| {
        db.execute(
            "UPDATE devices SET name=? WHERE id=?",
            rusqlite::params![name, id],
        )
    })? == 0
    {
        return Err(ApiError::not_found("device not found"));
    }
    let workspace: Option<String> = state.db.with_connection(|db| {
        use rusqlite::OptionalExtension;
        db.query_row(
            "SELECT workspace_id FROM devices WHERE id=?",
            [&id],
            |row| row.get(0),
        )
        .optional()
    })?;
    state.events.emit(
        &state.db,
        "device.renamed",
        workspace.as_deref(),
        Some(&user.id),
        Some(&id),
        serde_json::json!({"name":name}),
    )?;
    Ok(redirect(next(&values, &format!("/devices/{id}"))))
}

fn user(state: &AppState, headers: &HeaderMap) -> Result<crate::UserIdentity, ApiError> {
    browser_user(state, headers)
        .map_err(|_| ApiError::unauthorized("authentication required"))?
        .ok_or_else(|| ApiError::unauthorized("authentication required"))
}
fn owner_workspace(state: &AppState, user: &str, id: &str) -> Result<(), ApiError> {
    if workspace_role(state, user, id)?.as_deref() == Some("owner") {
        Ok(())
    } else {
        Err(ApiError::forbidden("owner required"))
    }
}
fn form(body: &[u8]) -> std::collections::HashMap<String, String> {
    url::form_urlencoded::parse(body).into_owned().collect()
}
fn clean(value: &str, error: &'static str) -> Result<String, ApiError> {
    let value = value.trim().chars().take(120).collect::<String>();
    if value.is_empty() {
        Err(ApiError::bad_request(error))
    } else {
        Ok(value)
    }
}
fn next<'a>(values: &'a std::collections::HashMap<String, String>, fallback: &'a str) -> &'a str {
    values
        .get("next")
        .filter(|value| value.starts_with('/') && !value.starts_with("//"))
        .map(String::as_str)
        .unwrap_or(fallback)
}
fn redirect(location: &str) -> Response {
    (StatusCode::SEE_OTHER, [("location", location.to_owned())]).into_response()
}
