use axum::{
    Json,
    http::StatusCode,
    response::{IntoResponse, Response},
};

#[derive(Debug)]
pub(crate) struct ApiError(pub(crate) StatusCode, pub(crate) String);

impl ApiError {
    pub(crate) fn bad_request(value: &'static str) -> Self {
        Self(StatusCode::BAD_REQUEST, value.into())
    }
    pub(crate) fn bad_request_owned(error: anyhow::Error) -> Self {
        Self(StatusCode::BAD_REQUEST, error.to_string())
    }
    pub(crate) fn unauthorized(value: &'static str) -> Self {
        Self(StatusCode::UNAUTHORIZED, value.into())
    }
    pub(crate) fn forbidden(value: &'static str) -> Self {
        Self(StatusCode::FORBIDDEN, value.into())
    }
    pub(crate) fn not_found(value: &'static str) -> Self {
        Self(StatusCode::NOT_FOUND, value.into())
    }
    pub(crate) fn conflict(value: &'static str) -> Self {
        Self(StatusCode::CONFLICT, value.into())
    }
    pub(crate) fn gone(value: &'static str) -> Self {
        Self(StatusCode::GONE, value.into())
    }
    pub(crate) fn bad_gateway(value: &'static str) -> Self {
        Self(StatusCode::BAD_GATEWAY, value.into())
    }
}

impl IntoResponse for ApiError {
    fn into_response(self) -> Response {
        (self.0, Json(serde_json::json!({"error":self.1}))).into_response()
    }
}

impl From<rusqlite::Error> for ApiError {
    fn from(_: rusqlite::Error) -> Self {
        Self(StatusCode::INTERNAL_SERVER_ERROR, "database error".into())
    }
}

impl From<anyhow::Error> for ApiError {
    fn from(error: anyhow::Error) -> Self {
        tracing::error!(%error, "request failed");
        Self(StatusCode::INTERNAL_SERVER_ERROR, "internal error".into())
    }
}

impl From<crate::AuthError> for ApiError {
    fn from(error: crate::AuthError) -> Self {
        Self(error.status(), error.to_string())
    }
}
