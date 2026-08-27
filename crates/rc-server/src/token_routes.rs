use crate::auth_public_routes::ApiError;
use crate::{
    API_DEFAULT_LIFETIME, AppState, auth_lifetime, consume_step_up, now_ms, require_browser,
};
use axum::{
    Json, Router,
    extract::{Path, State},
    http::HeaderMap,
    routing::{delete, get},
};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use serde::Deserialize;
use uuid::Uuid;

const API_SCOPES: [&str; 4] = ["read", "execute", "manage-devices", "manage-workspaces"];

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/tokens", get(list).post(create))
        .route("/api/v1/tokens/{id}", delete(remove))
}

async fn list(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let principal = require_browser(&state, &headers).map_err(auth_error)?;
    let rows = state.db.with_connection(|db| {
        let mut statement = db.prepare(
            "SELECT id,name,public_key,scopes,created_at,expires_at,last_used \
             FROM clients WHERE user_id=? AND kind='api' ORDER BY created_at DESC",
        )?;
        statement
            .query_map([principal.user.id], |row| {
                let scopes: String = row.get(3)?;
                Ok(serde_json::json!({
                    "id": row.get::<_, String>(0)?,
                    "name": row.get::<_, String>(1)?,
                    "public_key": row.get::<_, String>(2)?,
                    "scopes": serde_json::from_str::<Vec<String>>(&scopes).unwrap_or_default(),
                    "created_at": row.get::<_, i64>(4)?,
                    "expires_at": row.get::<_, i64>(5)?,
                    "last_used": row.get::<_, Option<i64>>(6)?,
                }))
            })?
            .collect::<Result<Vec<_>, _>>()
    })?;
    Ok(Json(serde_json::json!({"tokens": rows})))
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct CreateToken {
    name: Option<String>,
    scopes: Option<Vec<String>>,
    public_key: String,
    lifetime: Option<String>,
}

async fn create(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(input): Json<CreateToken>,
) -> Result<Json<serde_json::Value>, ApiError> {
    let principal = require_browser(&state, &headers).map_err(auth_error)?;
    consume_step_up(&state, &headers, &principal.user.id)
        .map_err(|_| ApiError::unauthorized("fresh passkey verification required"))?;
    if URL_SAFE_NO_PAD
        .decode(&input.public_key)
        .map(|v| v.len())
        .unwrap_or(0)
        != 32
    {
        return Err(ApiError::bad_request("invalid API signing key"));
    }
    let count = state.db.with_connection(|db| {
        db.query_row(
            "SELECT count(*) FROM clients WHERE user_id=? AND kind='api'",
            [&principal.user.id],
            |row| row.get::<_, i64>(0),
        )
    })?;
    if count >= 10 {
        return Err(ApiError::conflict("API key limit reached (10)"));
    }
    let scopes = normalize_scopes(input.scopes);
    let name = input
        .name
        .unwrap_or_else(|| "API key".into())
        .trim()
        .chars()
        .take(80)
        .collect::<String>();
    let name = if name.is_empty() {
        "API key".to_owned()
    } else {
        name
    };
    let lifetime = auth_lifetime(
        input.lifetime.as_deref(),
        API_DEFAULT_LIFETIME,
        true,
        now_ms(),
    )
    .map_err(ApiError::bad_request)?;
    let id = Uuid::new_v4().to_string();
    let created_at = now_ms();
    let scopes_json = serde_json::to_string(&scopes).map_err(anyhow::Error::from)?;
    state.db.with_connection(|db| {
        db.execute(
            "INSERT INTO clients(id,user_id,kind,name,public_key,scopes,created_at,expires_at) \
             VALUES(?,?,'api',?,?,?,?,?)",
            rusqlite::params![
                id,
                principal.user.id,
                name,
                input.public_key,
                scopes_json,
                created_at,
                lifetime.expires_at,
            ],
        )?;
        Ok(())
    })?;
    Ok(Json(serde_json::json!({
        "id": id,
        "publicKey": input.public_key,
        "scopes": scopes,
        "createdAt": created_at,
        "expiresAt": lifetime.expires_at,
    })))
}

async fn remove(
    State(state): State<AppState>,
    Path(id): Path<String>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let principal = require_browser(&state, &headers).map_err(auth_error)?;
    consume_step_up(&state, &headers, &principal.user.id)
        .map_err(|_| ApiError::unauthorized("fresh passkey verification required"))?;
    let changed = state.db.with_connection(|db| {
        db.execute(
            "DELETE FROM clients WHERE id=? AND user_id=? AND kind='api'",
            rusqlite::params![id, principal.user.id],
        )
    })?;
    if changed == 0 {
        return Err(ApiError::not_found("API key not found"));
    }
    Ok(Json(serde_json::json!({"ok": true})))
}

fn normalize_scopes(value: Option<Vec<String>>) -> Vec<String> {
    let requested = value.unwrap_or_else(|| vec!["read".into(), "execute".into()]);
    let mut scopes: Vec<_> = API_SCOPES
        .iter()
        .filter(|scope| requested.iter().any(|v| v == **scope))
        .map(|v| (*v).to_owned())
        .collect();
    if scopes.is_empty() {
        scopes.push("read".into());
    }
    scopes
}

fn auth_error(error: crate::RequestAuthError) -> ApiError {
    match error {
        crate::RequestAuthError::Unauthorized | crate::RequestAuthError::BrowserRequired => {
            ApiError::unauthorized("browser session required")
        }
        crate::RequestAuthError::Scope(scope) => ApiError::forbidden(scope),
        crate::RequestAuthError::Internal => ApiError(
            axum::http::StatusCode::INTERNAL_SERVER_ERROR,
            "database error".into(),
        ),
    }
}
