mod control;
mod enrollment;
mod lock;
mod node_auth;
mod process;
mod run_lock;
mod runtime;
mod state;
mod transport;
mod update;

pub use control::*;
pub use enrollment::*;
pub use lock::*;
pub use node_auth::*;
pub use process::*;
pub use run_lock::*;
pub use runtime::*;
pub use state::*;
pub use transport::*;
pub use update::*;

pub const DEFAULT_SERVER: &str = "https://rc.ohrats.party";
pub const NODE_CAPABILITIES: &[&str] = &["process", "update", "lock", "e2e", "webrtc"];
