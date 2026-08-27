mod oauth;
mod oauth_store;
mod page;
mod rpc;

use crate::AppState;
use axum::{
    Router,
    routing::{delete, get, post},
};

pub fn routes() -> Router<AppState> {
    Router::new()
        .route(
            "/.well-known/oauth-protected-resource",
            get(oauth::protected_metadata),
        )
        .route(
            "/.well-known/oauth-protected-resource/mcp",
            get(oauth::protected_metadata),
        )
        .route(
            "/.well-known/oauth-authorization-server",
            get(oauth::oauth_metadata),
        )
        .route("/oauth/register", post(oauth::register))
        .route("/oauth/authorize", get(oauth::authorize))
        .route("/oauth/authorize/prepare", post(oauth::prepare))
        .route("/oauth/authorize/approve", post(oauth::approve))
        .route("/oauth/authorize/cancel", post(oauth::cancel))
        .route(
            "/oauth/authorize/switch-account",
            post(oauth::switch_account),
        )
        .route("/oauth/token", post(oauth::token))
        .route("/oauth/grants/{id}", delete(oauth::revoke))
        .route("/mcp", post(rpc::mcp))
}
