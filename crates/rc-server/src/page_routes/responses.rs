use crate::{AppState, page_html::PageContext};
use axum::{
    http::StatusCode,
    response::{Html, IntoResponse, Response},
};

pub(super) fn redirect(location: &str) -> Response {
    (StatusCode::SEE_OTHER, [("location", location.to_owned())]).into_response()
}

pub(super) fn not_found(context: &PageContext) -> Response {
    (
        StatusCode::NOT_FOUND,
        Html(crate::page_html::authenticated_not_found(context)),
    )
        .into_response()
}

pub(super) fn public_not_found(state: &AppState) -> Response {
    let signup = crate::active_user_count(state).unwrap_or_default() > 0
        && state.config.public_signup
        && state.config.turnstile_site_key.is_some()
        && state.config.turnstile_secret_key.is_some();
    (
        StatusCode::NOT_FOUND,
        Html(crate::page_html::public_not_found(
            &state.config.public_url,
            signup,
        )),
    )
        .into_response()
}

pub(super) fn internal(error: anyhow::Error) -> Response {
    tracing::error!(%error, "page rendering failed");
    (
        StatusCode::INTERNAL_SERVER_ERROR,
        Html(crate::page_html::error(500, "Internal server error")),
    )
        .into_response()
}
