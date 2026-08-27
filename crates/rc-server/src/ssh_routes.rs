mod bridge;
mod keys;
mod tunnel;

use crate::auth_public_routes::ApiError;
use crate::{AppState, require_principal};
use axum::{
    Router,
    http::{HeaderMap, Method},
    routing::{delete, get},
};

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/ssh/keys", get(keys::list).post(keys::create))
        .route("/api/v1/ssh/keys/{id}", delete(keys::remove))
        .route("/api/v1/ssh/tunnel", get(tunnel::tunnel))
}

pub fn internal_routes() -> Router<AppState> {
    Router::new()
        .route("/authorized", get(keys::authorized))
        .route("/bridge", get(bridge::bridge))
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
