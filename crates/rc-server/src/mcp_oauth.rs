mod client;
mod grant;
mod token;

pub use client::{create_oauth_request, register_mcp_client};
pub use grant::{ApprovedGrant, PreparedGrant, approve_oauth_grant, prepare_oauth_grant};
pub use token::{access_grant, exchange_token, revoke_mcp_grant};

use crate::AppState;

pub const MCP_PROTOCOL_VERSION: &str = "2026-07-28";
pub const MCP_SCOPES: [&str; 2] = ["mcp:observe", "mcp:terminal"];

#[derive(Debug, Clone)]
pub struct McpGrantRecord {
    pub id: String,
    pub user_id: String,
    pub client_id: String,
    pub name: String,
    pub grant: String,
    pub grant_signature: String,
    pub client_control_id: String,
    pub credential_id: String,
    pub control_grant: String,
    pub control_assertion: String,
    pub expires_at: i64,
}

pub fn mcp_resource(state: &AppState) -> String {
    format!("{}/mcp", state.config.public_url.trim_end_matches('/'))
}
