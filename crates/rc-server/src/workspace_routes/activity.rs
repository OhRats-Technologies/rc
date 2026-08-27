use super::principal;
use crate::auth_public_routes::ApiError;
use crate::{AppState, workspace_role};
use axum::{
    Json,
    extract::{Path, State},
    http::{HeaderMap, Method},
};

pub(super) async fn get(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let path = format!("/api/v1/workspaces/{id}/activity");
    let principal = principal(&state, &headers, &Method::GET, &path, &[], Some("read"))?;
    if workspace_role(&state, &principal.user.id, &id)?.is_none() {
        return Err(ApiError::forbidden("forbidden"));
    }
    let rows = state.db.with_connection(|db| {
        let mut statement = db.prepare(
            "SELECT id,kind,detail,device_id,created_at FROM events WHERE workspace_id=? ORDER BY created_at DESC LIMIT 100",
        )?;
        statement
            .query_map([id], |row| {
                let detail: String = row.get(2)?;
                Ok(serde_json::json!({
                    "id": row.get::<_, i64>(0)?,
                    "kind": row.get::<_, String>(1)?,
                    "detail": serde_json::from_str::<serde_json::Value>(&detail).unwrap_or_default(),
                    "device_id": row.get::<_, Option<String>>(3)?,
                    "created_at": row.get::<_, i64>(4)?,
                }))
            })?
            .collect::<Result<Vec<_>, _>>()
    })?;
    Ok(Json(serde_json::json!({"events": rows})))
}
