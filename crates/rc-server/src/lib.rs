mod account_routes;
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

pub use auth_session::*;
pub use authority::*;
pub use cli_authorization::*;
pub use client_auth::*;
pub use config::*;
pub use control_authorization::*;
pub use control_hub::*;
pub use db::*;
pub use event_hub::*;
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
use std::{net::IpAddr, sync::Arc};
use webauthn_rs::prelude::{Url, Webauthn, WebauthnBuilder};

#[derive(Clone)]
pub struct AppState {
    pub config: Arc<Config>,
    pub db: Database,
    pub turn: TurnProvider,
    pub nodes: NodeHub,
    pub control: ControlHub,
    pub webauthn: Arc<Webauthn>,
    pub events: EventHub,
    pub ssh: SshHub,
    pub mcp: McpHub,
    pub rate_limits: middleware::RateLimiter,
}

impl AppState {
    pub fn new(config: Config) -> anyhow::Result<Self> {
        let webauthn = build_webauthn(&config.public_url)?;
        let db = Database::open(&config.db_path)?;
        let turn = TurnProvider::new(config.turn_token_id.clone(), config.turn_api_token.clone());
        let nodes = NodeHub::default();
        let control = ControlHub::new(nodes.clone(), turn.clone());
        let events = EventHub::default();
        let ssh = SshHub::default();
        let mcp = McpHub::default();
        Ok(Self {
            config: Arc::new(config),
            db,
            turn,
            nodes,
            control,
            webauthn: Arc::new(webauthn),
            events,
            ssh,
            mcp,
            rate_limits: middleware::RateLimiter::default(),
        })
    }

    pub fn release_device_sessions(&self, device_id: &str) {
        self.control.release_device(device_id);
        self.ssh.release_device(device_id);
        self.mcp.release_device(device_id);
    }

    pub async fn disconnect_device(&self, device_id: &str) {
        self.nodes.remove_if(device_id, "").await;
        self.release_device_sessions(device_id);
    }

    pub fn emit_device_presence(&self, device_id: &str, online: bool) {
        let workspace = match self.db.device_workspace(device_id) {
            Ok(Some(value)) => value,
            Ok(None) => return,
            Err(error) => {
                tracing::warn!(%device_id, %error, "failed to resolve device workspace");
                return;
            }
        };
        let kind = if online {
            "device.online"
        } else {
            "device.offline"
        };
        if let Err(error) = self.events.emit(
            &self.db,
            kind,
            Some(&workspace),
            None,
            Some(device_id),
            serde_json::json!({}),
        ) {
            tracing::warn!(%device_id, %error, "failed to emit device presence");
        }
    }
}

fn build_webauthn(public_url: &str) -> anyhow::Result<Webauthn> {
    let public_url = public_url.trim_end_matches('/');
    let origin = Url::parse(public_url)
        .map_err(|error| anyhow::anyhow!("PUBLIC_URL must be an absolute URL: {error}"))?;
    if !matches!(origin.scheme(), "http" | "https") {
        anyhow::bail!("PUBLIC_URL must use http or https");
    }
    let rp_id = origin
        .host_str()
        .ok_or_else(|| anyhow::anyhow!("PUBLIC_URL must contain a host"))?;
    if rp_id.parse::<IpAddr>().is_ok() {
        anyhow::bail!(
            "PUBLIC_URL must use a DNS hostname for passkeys; use http://localhost for local development instead of an IP address"
        );
    }
    WebauthnBuilder::new(rp_id, &origin)
        .map_err(|error| {
            anyhow::anyhow!(
                "PUBLIC_URL is not a valid passkey origin; use HTTPS with a DNS hostname, or http://localhost for local development: {error}"
            )
        })?
        .rp_name("RC")
        .build()
        .map_err(|error| anyhow::anyhow!("failed to configure passkeys: {error}"))
}

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
    use super::build_webauthn;

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
