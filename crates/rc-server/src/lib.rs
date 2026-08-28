mod account_routes;
mod app_state;
mod auth_private_routes;
mod auth_public_routes;
mod auth_session;
mod authority;
mod cli_authorization;
mod client_auth;
mod config;
mod control_authorization;
mod control_hub;
mod control_routes;
mod db;
mod device_process_routes;
mod event_hub;
mod event_routes;
mod execution;
mod lifetimes;
mod mcp_hub;
mod mcp_oauth;
mod mcp_routes;
mod mcp_tools;
mod middleware;
mod node_auth;
mod node_hub;
mod node_routes;
mod page_html;
mod page_routes;
mod request_auth;
mod resource_views;
mod ssh_hub;
mod ssh_routes;
mod step_up;
mod token_routes;
mod turn;
mod webauthn_flow;
mod webrtc_util;
mod workspace_authority_routes;
mod workspace_routes;

pub use app_state::*;
pub use auth_session::*;
pub use authority::*;
pub use cli_authorization::*;
pub use client_auth::*;
pub use config::*;
pub use control_authorization::*;
pub use control_hub::*;
pub use db::*;
pub use event_hub::*;
pub use execution::*;
pub use lifetimes::*;
pub use mcp_hub::*;
pub use mcp_oauth::*;
pub use node_hub::*;
pub use request_auth::*;
pub use resource_views::*;
pub use ssh_hub::*;
pub use step_up::*;
pub use turn::*;
pub use webauthn_flow::*;

use axum::{Router, routing::get};

pub fn app(state: AppState) -> Router {
    let static_dir = state.config.static_dir.clone();
    let middleware_state = state.clone();
    let request_id = http::HeaderName::from_static("x-request-id");
    let assets = tower::ServiceBuilder::new()
        .layer(tower_http::set_header::SetResponseHeaderLayer::overriding(
            http::header::CACHE_CONTROL,
            http::HeaderValue::from_static("public, max-age=0, must-revalidate"),
        ))
        .service(tower_http::services::ServeDir::new(static_dir));
    Router::new()
        .route("/healthz", get(|| async { "ok" }))
        .merge(auth_public_routes::routes())
        .merge(auth_private_routes::routes())
        .merge(account_routes::routes())
        .merge(token_routes::routes())
        .merge(workspace_routes::routes())
        .merge(workspace_authority_routes::routes())
        .merge(device_process_routes::routes())
        .merge(event_routes::routes())
        .merge(ssh_routes::routes())
        .merge(mcp_routes::routes())
        .merge(page_routes::routes())
        .nest_service("/assets", assets)
        .merge(control_routes::routes())
        .merge(node_routes::routes())
        .layer(axum::extract::DefaultBodyLimit::max(2 * 1024 * 1024))
        .layer(axum::middleware::from_fn_with_state(
            middleware_state,
            middleware::request_policy,
        ))
        .layer(tower_http::request_id::PropagateRequestIdLayer::new(
            request_id.clone(),
        ))
        .layer(tower_http::trace::TraceLayer::new_for_http())
        .layer(tower_http::request_id::SetRequestIdLayer::new(
            request_id,
            tower_http::request_id::MakeRequestUuid,
        ))
        .with_state(state)
}

pub fn ssh_internal_app(state: AppState) -> Router {
    ssh_routes::internal_routes().with_state(state)
}

#[cfg(test)]
mod tests {
    use super::app_state::build_webauthn;

    #[test]
    fn passkey_origin_accepts_localhost() {
        assert!(build_webauthn("http://localhost:3000").is_ok());
    }

    #[test]
    fn passkey_origin_rejects_ip_addresses_with_actionable_error() {
        let error = build_webauthn("http://127.0.0.1:3000").expect_err("IP origin should fail");
        assert!(error.to_string().contains("DNS hostname"));
    }
}
