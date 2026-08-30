mod authority;
mod control;
mod node;
mod node_transport;

pub use authority::*;
pub use control::*;
pub use node::*;
pub use node_transport::*;

pub const PROTOCOL_VERSION: u32 = 1;
pub const NODE_CONTROL_MESSAGE_LIMIT: usize = 1_048_576;
pub const PROCESS_INPUT_CHUNK_LIMIT: usize = 128 * 1024;
pub const MCP_IMAGE_CHUNK_BYTES: usize = 512 * 1024;
pub const MCP_IMAGE_MAX_BYTES: usize = 8 * 1024 * 1024;
