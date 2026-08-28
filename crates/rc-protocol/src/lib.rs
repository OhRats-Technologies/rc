mod authority;
mod control;
mod node;

pub use authority::*;
pub use control::*;
pub use node::*;

pub const PROTOCOL_VERSION: u32 = 1;
pub const NODE_CONTROL_MESSAGE_LIMIT: usize = 1_048_576;
pub const PROCESS_INPUT_CHUNK_LIMIT: usize = 128 * 1024;
