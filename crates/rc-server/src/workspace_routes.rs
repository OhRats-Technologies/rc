mod access;
mod activity;
mod enrollment;
mod membership;
mod workspace;

pub(crate) use membership::leave_workspace;

use crate::auth_public_routes::ApiError;
use crate::{AppState, require_principal, workspace_role};
use axum::{
    Router,
    http::{HeaderMap, Method},
    routing::{delete, get, patch, post},
};

const INVITE_TTL_MS: i64 = 4 * 60 * 60 * 1000;
const ENROLLMENT_TTL_MS: i64 = 30 * 60 * 1000;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/me", get(workspace::me))
        .route(
            "/api/v1/workspaces",
            get(workspace::list).post(workspace::create),
        )
        .route("/api/v1/workspaces/join", post(membership::join))
        .route(
            "/api/v1/workspaces/{id}",
            get(workspace::detail)
                .patch(workspace::rename)
                .delete(workspace::remove),
        )
        .route("/api/v1/workspaces/{id}/leave", post(membership::leave))
        .route("/api/v1/workspaces/{id}/activity", get(activity::get))
        .route("/api/v1/workspaces/{id}/invites", post(access::invite))
        .route("/api/v1/workspaces/{id}/access", get(access::access))
        .route(
            "/api/v1/workspaces/{id}/members/{member}",
            patch(access::member_role).delete(access::member_remove),
        )
        .route(
            "/api/v1/workspaces/{id}/invites/{invite}",
            delete(access::invite_remove),
        )
        .route(
            "/api/v1/workspaces/{id}/enrollments",
            post(enrollment::enrollment),
        )
}

fn principal(
    state: &AppState,
    headers: &HeaderMap,
    method: &Method,
    path: &str,
    body: &[u8],
    scope: Option<&'static str>,
) -> Result<crate::AuthPrincipal, ApiError> {
    require_principal(state, headers, method, path, body, scope)
        .map_err(|error| ApiError(error.status(), error.to_string()))
}

fn owner(state: &AppState, user: &str, id: &str) -> Result<(), ApiError> {
    if workspace_role(state, user, id)?.as_deref() != Some("owner") {
        return Err(ApiError::forbidden("owner required"));
    }
    Ok(())
}

fn parse<T: serde::de::DeserializeOwned>(body: &[u8]) -> Result<T, ApiError> {
    serde_json::from_slice(body).map_err(|_| ApiError::bad_request("invalid request"))
}

fn clean(value: &str, message: &'static str) -> Result<String, ApiError> {
    let value = value.trim().chars().take(120).collect::<String>();
    if value.is_empty() {
        Err(ApiError::bad_request(message))
    } else {
        Ok(value)
    }
}
