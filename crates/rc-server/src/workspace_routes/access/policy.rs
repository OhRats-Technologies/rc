use crate::auth_public_routes::ApiError;
use crate::{AppState, consume_step_up};
use axum::http::HeaderMap;

pub(super) enum RoleOutcome {
    Updated(String),
    OwnerRequired,
    Missing,
    LastOwner,
}

pub(super) enum RemoveOutcome {
    Removed(String),
    OwnerRequired,
    Missing,
    LastOwner,
}

pub(super) fn browser_step_up(
    state: &AppState,
    headers: &HeaderMap,
    principal: &crate::AuthPrincipal,
) -> Result<(), ApiError> {
    if !principal.browser {
        return Err(ApiError::unauthorized("browser session required"));
    }
    consume_step_up(state, headers, &principal.user.id)
        .map_err(|_| ApiError::unauthorized("fresh passkey verification required"))
}
