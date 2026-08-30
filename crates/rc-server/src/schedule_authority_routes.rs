use crate::auth_public_routes::ApiError;
use crate::{
    AppState, authority_hash, canonical_authority, control_proof, fresh_control_proof, now_ms,
    require_principal, workspace_role,
};
use axum::{
    Json, Router,
    body::Bytes,
    extract::{Path, State},
    http::{HeaderMap, Method},
    routing::put,
};
use serde::Deserialize;

const FRESH_AUTHORITY_MS: i64 = 5 * 60_000;
const MAX_RUNTIME_MS: u64 = 30 * 24 * 60 * 60_000;

pub fn routes() -> Router<AppState> {
    Router::new().route(
        "/api/v1/workspaces/{workspace}/schedule-grants/{schedule}",
        put(upsert).delete(remove),
    )
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct UpsertInput {
    client_id: String,
    device_id: String,
    spec_hash: String,
    max_runtime_ms: u64,
    #[serde(default)]
    expires_at: i64,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct RemoveInput {
    client_id: String,
}

async fn upsert(
    State(state): State<AppState>,
    Path((workspace, schedule)): Path<(String, String)>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<Json<serde_json::Value>, ApiError> {
    let path = format!("/api/v1/workspaces/{workspace}/schedule-grants/{schedule}");
    let principal = authenticate(&state, &headers, &Method::PUT, &path, &body, &workspace)?;
    let input: UpsertInput =
        serde_json::from_slice(&body).map_err(|_| ApiError::bad_request("invalid request"))?;
    validate(&schedule, &input)?;
    require_fresh_control(&state, &principal.user.id, &input.client_id)?;
    let changed = state.db.with_connection(|db| {
        db.execute(
            "INSERT INTO schedule_grants(schedule_id,workspace_id,device_id,user_id,spec_hash,max_runtime_ms,expires_at,created_at) SELECT ?,?,?,?,?,?,?,? WHERE EXISTS(SELECT 1 FROM devices WHERE id=? AND workspace_id=?) ON CONFLICT(schedule_id) DO UPDATE SET device_id=excluded.device_id,user_id=excluded.user_id,spec_hash=excluded.spec_hash,max_runtime_ms=excluded.max_runtime_ms,expires_at=excluded.expires_at,created_at=excluded.created_at WHERE schedule_grants.workspace_id=excluded.workspace_id",
            rusqlite::params![schedule, workspace, input.device_id, principal.user.id, input.spec_hash, input.max_runtime_ms as i64, input.expires_at, now_ms(), input.device_id, workspace],
        )
    })?;
    if changed != 1 {
        return Err(ApiError::not_found("device or schedule not found"));
    }
    response(&state, &workspace, &schedule)
}

async fn remove(
    State(state): State<AppState>,
    Path((workspace, schedule)): Path<(String, String)>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<Json<serde_json::Value>, ApiError> {
    let path = format!("/api/v1/workspaces/{workspace}/schedule-grants/{schedule}");
    let principal = authenticate(&state, &headers, &Method::DELETE, &path, &body, &workspace)?;
    let input: RemoveInput =
        serde_json::from_slice(&body).map_err(|_| ApiError::bad_request("invalid request"))?;
    require_control(&state, &principal.user.id, &input.client_id)?;
    let changed = state.db.with_connection(|db| {
        db.execute(
            "DELETE FROM schedule_grants WHERE schedule_id=? AND workspace_id=?",
            rusqlite::params![schedule, workspace],
        )
    })?;
    if changed != 1 {
        return Err(ApiError::not_found("schedule grant not found"));
    }
    response(&state, &workspace, &schedule)
}

fn authenticate(
    state: &AppState,
    headers: &HeaderMap,
    method: &Method,
    path: &str,
    body: &[u8],
    workspace: &str,
) -> Result<crate::AuthPrincipal, ApiError> {
    let principal = require_principal(state, headers, method, path, body, None)
        .map_err(|error| ApiError(error.status(), error.to_string()))?;
    if workspace_role(state, &principal.user.id, workspace)?.as_deref() != Some("owner") {
        return Err(ApiError::forbidden("owner required"));
    }
    Ok(principal)
}

fn require_fresh_control(state: &AppState, user: &str, client: &str) -> Result<(), ApiError> {
    if fresh_control_proof(state, user, client, FRESH_AUTHORITY_MS)?.is_none() {
        return Err(ApiError::unauthorized(
            "fresh passkey-backed control authorization required",
        ));
    }
    Ok(())
}

fn require_control(state: &AppState, user: &str, client: &str) -> Result<(), ApiError> {
    if control_proof(state, user, client)?.is_none() {
        return Err(ApiError::unauthorized(
            "passkey-backed control authorization required",
        ));
    }
    Ok(())
}

fn validate(schedule: &str, input: &UpsertInput) -> Result<(), ApiError> {
    let hash_ok = input.spec_hash.len() == 64
        && input
            .spec_hash
            .bytes()
            .all(|value| value.is_ascii_hexdigit());
    if schedule.trim().is_empty()
        || input.device_id.trim().is_empty()
        || !hash_ok
        || input.max_runtime_ms == 0
        || input.max_runtime_ms > MAX_RUNTIME_MS
        || input.expires_at < 0
        || (input.expires_at != 0 && input.expires_at <= now_ms())
    {
        return Err(ApiError::bad_request("invalid schedule authority permit"));
    }
    Ok(())
}

fn response(
    state: &AppState,
    workspace: &str,
    schedule: &str,
) -> Result<Json<serde_json::Value>, ApiError> {
    let snapshot = canonical_authority(&state.db, workspace)?;
    Ok(Json(serde_json::json!({
        "ok": true,
        "scheduleId": schedule,
        "lockHash": authority_hash(&snapshot)
    })))
}
