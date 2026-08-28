use crate::AppState;
use axum::{
    extract::{Path, State},
    http::StatusCode,
    response::{Html, IntoResponse, Response},
};

pub(super) async fn index(State(state): State<AppState>) -> Html<String> {
    Html(render(&state, "quickstart").expect("quickstart documentation is present"))
}

pub(super) async fn topic(State(state): State<AppState>, Path(topic): Path<String>) -> Response {
    if topic == "quickstart" {
        return (StatusCode::PERMANENT_REDIRECT, [("location", "/docs")]).into_response();
    }
    let Some(page) = render(&state, &topic) else {
        return (
            StatusCode::NOT_FOUND,
            Html(crate::page_html::error(404, "Documentation not found")),
        )
            .into_response();
    };
    Html(page).into_response()
}

fn render(state: &AppState, topic: &str) -> Option<String> {
    crate::page_html::docs(
        topic,
        &state.config.public_url,
        public_signup_configured(state),
    )
}

fn public_signup_configured(state: &AppState) -> bool {
    state.config.public_signup
        && state.config.turnstile_site_key.is_some()
        && state.config.turnstile_secret_key.is_some()
}

pub(super) async fn install_script() -> Response {
    (
        [
            ("content-type", "text/x-shellscript; charset=utf-8"),
            ("cache-control", "no-cache"),
        ],
        include_str!("../../../../public/install.sh"),
    )
        .into_response()
}

pub(super) async fn robots() -> &'static str {
    "User-agent: *\nAllow: /\nDisallow: /devices\nDisallow: /account\nDisallow: /api/v1/auth/\nDisallow: /oauth/\nDisallow: /mcp\n"
}
