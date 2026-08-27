use super::{clean, parse, principal};
use crate::auth_public_routes::ApiError;
use crate::{AppState, devices_json, now_ms, workspace_json, workspaces_json};
use axum::{
    Json,
    body::Bytes,
    extract::{Path, State},
    http::{HeaderMap, Method, StatusCode},
};
use serde::Deserialize;
use uuid::Uuid;

pub(super) async fn me(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let p = principal(
        &state,
        &headers,
        &Method::GET,
        "/api/v1/me",
        &[],
        Some("read"),
    )?;
    Ok(Json(
        serde_json::json!({"user":p.user,"workspaces":workspaces_json(&state,&p.user.id)?}),
    ))
}

pub(super) async fn list(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let p = principal(
        &state,
        &headers,
        &Method::GET,
        "/api/v1/workspaces",
        &[],
        Some("read"),
    )?;
    Ok(Json(
        serde_json::json!({"workspaces":workspaces_json(&state,&p.user.id)?}),
    ))
}

#[derive(Deserialize)]
struct Name {
    name: String,
}
pub(super) async fn create(
    State(state): State<AppState>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<(StatusCode, Json<serde_json::Value>), ApiError> {
    let p = principal(
        &state,
        &headers,
        &Method::POST,
        "/api/v1/workspaces",
        &body,
        Some("manage-workspaces"),
    )?;
    let input: Name = parse(&body)?;
    let name = clean(&input.name, "workspace name required")?;
    let count = state.db.with_connection(|db| {
        db.query_row(
            "SELECT count(*) FROM workspaces WHERE created_by=?",
            [&p.user.id],
            |row| row.get::<_, i64>(0),
        )
    })?;
    if count >= 10 {
        return Err(ApiError::conflict("workspace limit reached (10)"));
    }
    let id = Uuid::new_v4().to_string();
    state.db.with_connection_mut(|db| {
        let tx = db.transaction()?;
        let created_at = now_ms();
        tx.execute(
            "INSERT INTO workspaces(id,name,created_by,created_at) VALUES(?,?,?,?)",
            rusqlite::params![id, name, p.user.id, created_at],
        )?;
        tx.execute(
            "INSERT INTO workspace_members(workspace_id,user_id,role,joined_at) VALUES(?,?,'owner',?)",
            rusqlite::params![id, p.user.id, created_at],
        )?;
        tx.commit()
    })?;
    state.events.emit(
        &state.db,
        "workspace.created",
        Some(&id),
        Some(&p.user.id),
        None,
        serde_json::json!({"name":name}),
    )?;
    Ok((StatusCode::CREATED, Json(serde_json::json!({"id":id}))))
}

pub(super) async fn detail(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let path = format!("/api/v1/workspaces/{id}");
    let p = principal(&state, &headers, &Method::GET, &path, &[], Some("read"))?;
    let workspace = workspace_json(&state, &p.user.id, &id)?
        .ok_or_else(|| ApiError::not_found("workspace not found"))?;
    let devices = devices_json(&state, &p.user.id)
        .await?
        .into_iter()
        .filter(|device| device["workspace_id"] == id)
        .collect::<Vec<_>>();
    Ok(Json(
        serde_json::json!({"workspace":workspace,"devices":devices}),
    ))
}

pub(super) async fn rename(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<Json<serde_json::Value>, ApiError> {
    let path = format!("/api/v1/workspaces/{id}");
    let p = principal(
        &state,
        &headers,
        &Method::PATCH,
        &path,
        &body,
        Some("manage-workspaces"),
    )?;
    super::owner(&state, &p.user.id, &id)?;
    let input: Name = parse(&body)?;
    let name = clean(&input.name, "workspace name required")?;
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
        Some(&p.user.id),
        None,
        serde_json::json!({"name":name}),
    )?;
    Ok(Json(serde_json::json!({"name":name})))
}

pub(super) async fn remove(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let path = format!("/api/v1/workspaces/{id}");
    let p = principal(
        &state,
        &headers,
        &Method::DELETE,
        &path,
        &[],
        Some("manage-workspaces"),
    )?;
    super::owner(&state, &p.user.id, &id)?;
    let devices = state.db.with_connection(|db| {
        let mut statement = db.prepare("SELECT id FROM devices WHERE workspace_id=?")?;
        statement
            .query_map([&id], |row| row.get::<_, String>(0))?
            .collect::<Result<Vec<_>, _>>()
    })?;
    for device in devices {
        let _ = state.db.revoke_device(&device)?;
        state.disconnect_device(&device).await;
    }
    if state
        .db
        .with_connection(|db| db.execute("DELETE FROM workspaces WHERE id=?", [&id]))?
        == 0
    {
        return Err(ApiError::not_found("workspace not found"));
    }
    Ok(Json(serde_json::json!({"ok":true})))
}
