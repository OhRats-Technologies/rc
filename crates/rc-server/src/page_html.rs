mod account;
mod auth;
mod devices;
mod docs;
mod document;
mod format;
mod landing;
mod misc;
mod public_snapshots;
mod sidebar;
mod status;

pub use account::{account, api_keys};
pub use auth::{AuthPage, auth};
pub use devices::{device, devices, process};
pub use docs::docs;
pub use document::{authenticated_document, authenticated_status_document, public_document};
pub use landing::landing;
pub use misc::{activity, cli_login, enroll, error, mcp_page, workspace_access};
pub use status::{authenticated_not_found, public_not_found};

use crate::UserIdentity;

#[derive(Clone)]
pub struct PageContext {
    pub user: UserIdentity,
    pub workspaces: Vec<serde_json::Value>,
    pub devices: Vec<serde_json::Value>,
    pub path: String,
    pub sidebar: String,
}

pub struct McpPageGrant {
    pub id: String,
    pub name: String,
    pub scopes: String,
    pub expires_at: i64,
    pub last_used: Option<i64>,
    pub device_count: usize,
}

pub fn esc(value: &str) -> String {
    value
        .replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
        .replace('"', "&quot;")
        .replace('\'', "&#39;")
}

pub(super) fn string(value: &serde_json::Value, key: &str) -> String {
    value
        .get(key)
        .and_then(serde_json::Value::as_str)
        .unwrap_or_default()
        .to_owned()
}

pub(super) fn bool_value(value: &serde_json::Value, key: &str) -> bool {
    value
        .get(key)
        .and_then(serde_json::Value::as_bool)
        .unwrap_or(false)
}

pub(super) fn integer(value: &serde_json::Value, key: &str) -> i64 {
    value
        .get(key)
        .and_then(serde_json::Value::as_i64)
        .unwrap_or_default()
}
