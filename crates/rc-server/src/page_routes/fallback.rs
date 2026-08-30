use super::responses::{internal, public_not_found};
use crate::AppState;
use axum::{
    extract::State,
    http::{HeaderMap, StatusCode, Uri},
    response::{Html, IntoResponse, Response},
};

pub(crate) async fn route(State(state): State<AppState>, headers: HeaderMap, uri: Uri) -> Response {
    if protocol_path(uri.path()) {
        return StatusCode::NOT_FOUND.into_response();
    }
    match super::context::load(&state, &headers, uri.path()).await {
        Ok(Some(context)) => (
            StatusCode::NOT_FOUND,
            Html(crate::page_html::authenticated_not_found(&context)),
        )
            .into_response(),
        Ok(None) => public_not_found(&state),
        Err(error) => internal(error),
    }
}

fn protocol_path(path: &str) -> bool {
    path.starts_with("/api/")
        || path == "/mcp"
        || path.starts_with("/mcp/")
        || path.starts_with("/oauth/")
        || path.starts_with("/.well-known/")
}
