use crate::auth_public_routes::ApiError;
use crate::{
    AppState, DELETED_USER_ID, browser_user, clear_session_cookie, consume_step_up,
    revoke_browser_session,
};
use axum::{
    Json, Router,
    body::Bytes,
    extract::State,
    http::{HeaderMap, HeaderValue, StatusCode},
    response::{IntoResponse, Response},
    routing::{patch, post},
};
use serde::Deserialize;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/account", patch(rename_api).delete(delete_api))
        .route("/account/name", post(rename_form))
        .route("/account/logout", post(logout_form))
        .route("/account/delete", post(delete_form))
}

#[derive(Deserialize)]
struct Rename {
    name: String,
}

async fn rename_api(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(input): Json<Rename>,
) -> Result<Json<serde_json::Value>, ApiError> {
    let user = user(&state, &headers)?;
    let name = rename(&state, &user.id, &input.name)?;
    Ok(Json(serde_json::json!({"name":name})))
}

async fn rename_form(
    State(state): State<AppState>,
    headers: HeaderMap,
    body: Bytes,
) -> Result<Response, ApiError> {
    let user = user(&state, &headers)?;
    let values = url::form_urlencoded::parse(&body)
        .into_owned()
        .collect::<std::collections::HashMap<_, _>>();
    let name = rename(
        &state,
        &user.id,
        values.get("name").map(String::as_str).unwrap_or_default(),
    )?;
    if headers
        .get("accept")
        .and_then(|v| v.to_str().ok())
        .is_some_and(|v| v.contains("application/json"))
    {
        Ok(Json(serde_json::json!({"name":name})).into_response())
    } else {
        Ok((StatusCode::SEE_OTHER, [("location", "/account")]).into_response())
    }
}

async fn delete_api(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Response, ApiError> {
    delete_authorized_user(&state, &headers).await?;
    let response = Json(serde_json::json!({"ok":true})).into_response();
    clear_cookie(&state, response)
}
async fn delete_form(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Response, ApiError> {
    delete_authorized_user(&state, &headers).await?;
    clear_cookie(
        &state,
        (StatusCode::SEE_OTHER, [("location", "/")]).into_response(),
    )
}
async fn logout_form(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Response, ApiError> {
    revoke_browser_session(&state, &headers)
        .map_err(|_| ApiError::unauthorized("authentication required"))?;
    clear_cookie(
        &state,
        (StatusCode::SEE_OTHER, [("location", "/")]).into_response(),
    )
}

async fn delete_authorized_user(state: &AppState, headers: &HeaderMap) -> Result<(), ApiError> {
    let user = user(state, headers)?;
    consume_step_up(state, headers, &user.id)
        .map_err(|_| ApiError::unauthorized("fresh passkey verification required"))?;
    delete_user(state, &user.id).await
}

fn rename(state: &AppState, user: &str, value: &str) -> Result<String, ApiError> {
    let name = value.trim().chars().take(120).collect::<String>();
    if name.is_empty() {
        return Err(ApiError::bad_request("account name required"));
    }
    if state.db.with_connection(|db| {
        db.execute(
            "UPDATE users SET name=? WHERE id=? AND id<>?",
            rusqlite::params![name, user, DELETED_USER_ID],
        )
    })? == 0
    {
        return Err(ApiError::not_found("account not found"));
    }
    Ok(name)
}

async fn delete_user(state: &AppState, user: &str) -> Result<(), ApiError> {
    let dispositions = workspace_dispositions(state, user)?;
    for disposition in dispositions.iter().filter(|value| value.delete) {
        let devices = state.db.with_connection(|db| {
            let mut statement = db.prepare("SELECT id FROM devices WHERE workspace_id=?")?;
            statement
                .query_map([&disposition.id], |row| row.get::<_, String>(0))?
                .collect::<Result<Vec<_>, _>>()
        })?;
        for device in devices {
            let _ = state.db.revoke_device(&device)?;
            state.disconnect_device(&device).await;
        }
    }
    state.db.with_connection_mut(|db| {
        let tx = db.transaction()?;
        tx.execute(
            "INSERT OR IGNORE INTO users(id,name,created_at) VALUES(?,'Deleted account',?)",
            rusqlite::params![DELETED_USER_ID, crate::now_ms()],
        )?;
        for disposition in &dispositions {
            if disposition.delete {
                tx.execute("DELETE FROM workspaces WHERE id=?", [&disposition.id])?;
            } else if let Some(replacement) = &disposition.replacement {
                tx.execute(
                    "UPDATE workspaces SET created_by=? WHERE id=? AND created_by=?",
                    rusqlite::params![replacement, disposition.id, user],
                )?;
            }
        }
        for table in ["workspace_invites", "enrollment_tokens", "processes"] {
            tx.execute(
                &format!("UPDATE {table} SET created_by=? WHERE created_by=?"),
                rusqlite::params![DELETED_USER_ID, user],
            )?;
        }
        tx.execute(
            "DELETE FROM users WHERE id=? AND id<>?",
            rusqlite::params![user, DELETED_USER_ID],
        )?;
        tx.commit()
    })?;
    Ok(())
}

struct WorkspaceDisposition {
    id: String,
    replacement: Option<String>,
    delete: bool,
}

fn workspace_dispositions(
    state: &AppState,
    user: &str,
) -> Result<Vec<WorkspaceDisposition>, ApiError> {
    Ok(state.db.with_connection(|db| {
        let mut statement = db.prepare(
            "SELECT w.id,w.created_by,(SELECT user_id FROM workspace_members replacement WHERE replacement.workspace_id=w.id AND replacement.role='owner' AND replacement.user_id<>? ORDER BY replacement.joined_at LIMIT 1) FROM workspaces w WHERE w.created_by=? OR EXISTS(SELECT 1 FROM workspace_members member WHERE member.workspace_id=w.id AND member.user_id=? AND member.role='owner')",
        )?;
        statement
            .query_map(rusqlite::params![user, user, user], |row| {
                let created_by = row.get::<_, String>(1)?;
                let replacement = row.get::<_, Option<String>>(2)?;
                Ok(WorkspaceDisposition {
                    id: row.get(0)?,
                    delete: replacement.is_none(),
                    replacement: (created_by == user).then_some(replacement).flatten(),
                })
            })?
            .collect::<Result<Vec<_>, _>>()
    })?)
}

fn clear_cookie(state: &AppState, mut response: Response) -> Result<Response, ApiError> {
    let cookie = clear_session_cookie(state);
    let value = HeaderValue::from_str(&cookie).map_err(|_| {
        ApiError(
            StatusCode::INTERNAL_SERVER_ERROR,
            "failed to clear browser session".into(),
        )
    })?;
    response.headers_mut().insert("set-cookie", value);
    Ok(response)
}
fn user(state: &AppState, headers: &HeaderMap) -> Result<crate::UserIdentity, ApiError> {
    browser_user(state, headers)
        .map_err(|_| ApiError::unauthorized("authentication required"))?
        .ok_or_else(|| ApiError::unauthorized("authentication required"))
}
