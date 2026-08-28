use axum::{
    http::StatusCode,
    response::{Html, IntoResponse, Response},
};

pub(super) fn redirect(location: &str) -> Response {
    (StatusCode::SEE_OTHER, [("location", location.to_owned())]).into_response()
}

pub(super) fn not_found() -> Response {
    (
        StatusCode::NOT_FOUND,
        Html(crate::page_html::error(404, "Not found")),
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
