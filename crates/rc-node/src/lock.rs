mod mcp;
mod persistence;
mod proof;
mod schedule;
mod sync;

pub use mcp::verify_mcp_grant;
pub use persistence::{bootstrap_lock, load_lock, lock_metadata, snapshot_hash};
pub use proof::{api_control_authority, hosted_control_authority, verify_control_proof};
pub use schedule::schedule_authority;
pub use sync::sync_lock;

use rc_protocol::ControlGrant;
use std::io;

#[derive(Debug, thiserror::Error)]
pub enum LockError {
    #[error("RC Lock is not initialized")]
    Missing,
    #[error("existing RC Lock is unreadable; refusing server bootstrap")]
    Corrupt,
    #[error("invalid RC Lock authority snapshot")]
    Snapshot,
    #[error("invalid RC server origin")]
    Origin,
    #[error("API control key rejected")]
    ApiKey,
    #[error("invalid control grant")]
    Grant,
    #[error("expired control grant")]
    GrantExpired,
    #[error("control credential is not authorized")]
    Credential,
    #[error("invalid passkey assertion")]
    Assertion,
    #[error("passkey credential mismatch")]
    CredentialMismatch,
    #[error("invalid stored passkey")]
    StoredPasskey,
    #[error("passkey grant verification failed")]
    Passkey,
    #[error("stale RC Lock authority transition")]
    StaleTransition,
    #[error("workspace authority mismatch")]
    WorkspaceMismatch,
    #[error("owner authorization required for RC Lock sync")]
    OwnerRequired,
    #[error("invalid RC Lock authority signature")]
    AuthoritySignature,
    #[error("MCP grant rejected")]
    McpGrant,
    #[error("MCP grant signature rejected")]
    McpSignature,
    #[error("schedule authority rejected")]
    ScheduleGrant,
    #[error("RC Lock generation exhausted")]
    GenerationExhausted,
    #[error(transparent)]
    Io(#[from] io::Error),
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ApiControlAuthority {
    pub user_id: String,
    pub role: String,
    pub public_key: String,
    pub can_execute: bool,
    pub can_manage_devices: bool,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ControlAuthority {
    pub grant: ControlGrant,
    pub role: String,
}

pub(crate) fn now_ms() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
}
