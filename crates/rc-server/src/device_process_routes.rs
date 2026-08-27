use crate::auth_public_routes::ApiError;
use crate::{
    AppState, device_json, devices_json, now_ms, process_json, processes_for_device,
    require_principal,
};
use axum::{
    Json, Router,
    body::Bytes,
    extract::{Path, State},
    http::{HeaderMap, Method, StatusCode},
    routing::get,
};
use serde::Deserialize;
use uuid::Uuid;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/devices", get(list))
        .route(
            "/api/v1/devices/{id}",
            get(detail).patch(rename).delete(remove),
        )
        .route(
            "/api/v1/devices/{id}/processes",
            get(processes).post(process_create),
        )
        .route("/api/v1/processes/{id}", get(process_detail))
}

async fn list(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let p = principal(
        &state,
        &headers,
        &Method::GET,
        "/api/v1/devices",
        &[],
        Some("read"),
    )?;
    Ok(Json(
        serde_json::json!({"devices":devices_json(&state,&p.user.id).await?}),
    ))
}

async fn detail(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let path = format!("/api/v1/devices/{id}");
    let p = principal(&state, &headers, &Method::GET, &path, &[], Some("read"))?;
    let device = device_json(&state, &p.user.id, &id)
        .await?
        .ok_or_else(|| ApiError::not_found("device not found"))?;
    Ok(Json(serde_json::json!({"device":device})))
}

#[derive(Deserialize)]
struct Rename {
    name: String,
}
async fn rename(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<Json<serde_json::Value>, ApiError> {
    let path = format!("/api/v1/devices/{id}");
    let p = principal(
        &state,
        &headers,
        &Method::PATCH,
        &path,
        &body,
        Some("manage-devices"),
    )?;
    require_owner(&state, &p.user.id, &id)?;
    let input: Rename =
        serde_json::from_slice(&body).map_err(|_| ApiError::bad_request("invalid request"))?;
    let name = input.name.trim().chars().take(120).collect::<String>();
    if name.is_empty() {
        return Err(ApiError::bad_request("device name required"));
    }
    if state.db.with_connection(|db| {
        db.execute(
            "UPDATE devices SET name=? WHERE id=?",
            rusqlite::params![name, id],
        )
    })? == 0
    {
        return Err(ApiError::not_found("device not found"));
    }
    let workspace = workspace_for_device(&state, &id)?;
    state.events.emit(
        &state.db,
        "device.renamed",
        workspace.as_deref(),
        Some(&p.user.id),
        Some(&id),
        serde_json::json!({"name":name}),
    )?;
    Ok(Json(serde_json::json!({"name":name})))
}

async fn remove(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let path = format!("/api/v1/devices/{id}");
    let p = principal(
        &state,
        &headers,
        &Method::DELETE,
        &path,
        &[],
        Some("manage-devices"),
    )?;
    require_owner(&state, &p.user.id, &id)?;
    let workspace = workspace_for_device(&state, &id)?;
    if !state.db.revoke_device(&id)? {
        return Err(ApiError::not_found("device not found"));
    }
    state.disconnect_device(&id).await;
    state.events.emit(
        &state.db,
        "device.removed",
        workspace.as_deref(),
        Some(&p.user.id),
        Some(&id),
        serde_json::json!({"deviceId":id}),
    )?;
    Ok(Json(serde_json::json!({"ok":true})))
}

async fn processes(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let path = format!("/api/v1/devices/{id}/processes");
    let p = principal(&state, &headers, &Method::GET, &path, &[], Some("read"))?;
    require_operator(&state, &p.user.id, &id)?;
    Ok(Json(
        serde_json::json!({"processes":processes_for_device(&state,&p.user.id,&id)?}),
    ))
}

#[derive(Default, Deserialize)]
struct CreateProcess {
    terminal: Option<bool>,
}
async fn process_create(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<(StatusCode, Json<serde_json::Value>), ApiError> {
    let path = format!("/api/v1/devices/{id}/processes");
    let p = principal(
        &state,
        &headers,
        &Method::POST,
        &path,
        &body,
        Some("execute"),
    )?;
    require_operator(&state, &p.user.id, &id)?;
    if !state.nodes.online(&id).await {
        return Err(ApiError::conflict("device is offline"));
    }
    let input = parse_process_request(&body)?;
    let origin = p
        .client
        .as_ref()
        .map(|c| c.kind.as_str())
        .filter(|v| matches!(*v, "api" | "cli"))
        .unwrap_or("browser");
    let process_id = Uuid::new_v4().to_string();
    let terminal = input.terminal.unwrap_or(false);
    state.db.with_connection(|db| {
        db.execute(
            "INSERT INTO processes(id,device_id,origin,status,terminal,created_by,created_at) \
             VALUES(?,?,?,'starting',?,?,?)",
            rusqlite::params![
                process_id,
                id,
                origin,
                i64::from(terminal),
                p.user.id,
                now_ms(),
            ],
        )?;
        Ok(())
    })?;
    let workspace = workspace_for_device(&state, &id)?;
    state.events.emit(
        &state.db,
        "process.created",
        workspace.as_deref(),
        Some(&p.user.id),
        Some(&id),
        serde_json::json!({"processId":process_id,"origin":origin,"terminal":terminal}),
    )?;
    Ok((
        StatusCode::CREATED,
        Json(serde_json::json!({"processId":process_id})),
    ))
}

fn parse_process_request(body: &[u8]) -> Result<CreateProcess, ApiError> {
    if body.is_empty() {
        return Ok(CreateProcess::default());
    }
    serde_json::from_slice(body).map_err(|_| ApiError::bad_request("invalid request"))
}

async fn process_detail(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let path = format!("/api/v1/processes/{id}");
    let p = principal(&state, &headers, &Method::GET, &path, &[], Some("read"))?;
    let process = process_json(&state, &p.user.id, &id)?
        .ok_or_else(|| ApiError::not_found("process not found"))?;
    let device = process["device_id"].as_str().unwrap_or_default().to_owned();
    require_operator(&state, &p.user.id, &device)?;
    Ok(Json(serde_json::json!({"process":process})))
}

fn principal(
    state: &AppState,
    h: &HeaderMap,
    m: &Method,
    path: &str,
    b: &[u8],
    scope: Option<&'static str>,
) -> Result<crate::AuthPrincipal, ApiError> {
    require_principal(state, h, m, path, b, scope).map_err(|e| ApiError(e.status(), e.to_string()))
}
fn role(state: &AppState, user: &str, device: &str) -> Result<Option<String>, ApiError> {
    Ok(state.db.device_role(user, device)?)
}
fn require_owner(state: &AppState, user: &str, device: &str) -> Result<(), ApiError> {
    match role(state, user, device)?.as_deref() {
        Some("owner") => Ok(()),
        None => Err(ApiError::not_found("device not found")),
        _ => Err(ApiError::forbidden("owner required")),
    }
}
fn require_operator(state: &AppState, user: &str, device: &str) -> Result<(), ApiError> {
    match role(state, user, device)?.as_deref() {
        Some("owner" | "operator") => Ok(()),
        None => Err(ApiError::not_found("device not found")),
        _ => Err(ApiError::forbidden("operator required")),
    }
}
fn workspace_for_device(state: &AppState, device: &str) -> Result<Option<String>, ApiError> {
    use rusqlite::OptionalExtension;
    Ok(state.db.with_connection(|db| {
        db.query_row(
            "SELECT workspace_id FROM devices WHERE id=?",
            [device],
            |r| r.get(0),
        )
        .optional()
    })?)
}

#[cfg(test)]
mod tests {
    use super::parse_process_request;

    #[test]
    fn process_request_accepts_empty_or_valid_json() {
        assert!(parse_process_request(&[]).is_ok());
        assert!(parse_process_request(br#"{"terminal":true}"#).is_ok());
    }

    #[test]
    fn process_request_rejects_malformed_json() {
        assert!(parse_process_request(br#"{"terminal":tru}"#).is_err());
    }
}
