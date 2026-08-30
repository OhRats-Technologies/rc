mod control;
mod enrollment;
mod lock;
mod mesh_authority;
mod node_auth;
mod policy;
mod process;
mod run_lock;
mod runtime;
mod schedule;
mod state;
mod transport;
mod update;

pub use control::*;
pub use enrollment::*;
pub use lock::*;
pub use mesh_authority::*;
pub use node_auth::*;
pub use policy::*;
pub use process::*;
pub use run_lock::*;
pub use runtime::*;
pub use schedule::*;
pub use state::*;
pub use transport::*;
pub use update::*;

pub const DEFAULT_SERVER: &str = "https://rc.ohrats.party";
pub const NODE_CAPABILITIES: &[&str] = &[
    "process",
    "execution-v2",
    "scheduler",
    "update",
    "lock",
    "e2e",
    "webrtc",
];
