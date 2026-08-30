mod image;
mod result;

use dashmap::DashMap;
use parking_lot::Mutex;
use rc_protocol::NodeToServer;
use std::sync::Arc;
use tokio::sync::oneshot;

pub use result::{McpOutputChunk, McpProcessResult};

const MAX_PENDING_STATUS_REQUESTS: usize = 4_096;

#[derive(Clone, Default)]
pub struct McpHub {
    images: Arc<DashMap<String, Arc<McpImagePending>>>,
    status_requests: Arc<DashMap<String, McpStatusPending>>,
    capacity: Arc<Mutex<()>>,
}

struct McpStatusPending {
    device_id: String,
    sender: oneshot::Sender<NodeToServer>,
}

pub(super) struct McpImagePending {
    device_id: String,
    bytes: Mutex<Vec<u8>>,
    sender: Mutex<Option<oneshot::Sender<Result<McpImage, String>>>>,
}

#[derive(Debug)]
pub struct McpImage {
    pub mime_type: String,
    pub bytes: Vec<u8>,
}

impl McpHub {
    pub fn handle(&self, device_id: &str, message: &NodeToServer) -> Option<(String, i32, String)> {
        match message {
            NodeToServer::McpExecutionStatusResult { request_id, .. }
            | NodeToServer::McpExecutionOperationResult { request_id, .. } => {
                let matches = self
                    .status_requests
                    .get(request_id)
                    .is_some_and(|pending| pending.device_id == device_id);
                if matches && let Some((_, pending)) = self.status_requests.remove(request_id) {
                    let _ = pending.sender.send(message.clone());
                }
                None
            }
            NodeToServer::McpImageChunk { request_id, data } => {
                self.image_chunk(device_id, request_id, data);
                None
            }
            NodeToServer::McpImageResult {
                request_id,
                mime_type,
                size_bytes,
                error,
            } => {
                self.image_finish(device_id, request_id, mime_type, *size_bytes, error);
                None
            }
            NodeToServer::McpExit {
                process_id,
                exit_code,
                signal,
            } => Some((process_id.clone(), *exit_code, signal.clone())),
            _ => None,
        }
    }

    pub fn status_request(
        &self,
        device_id: &str,
    ) -> anyhow::Result<(String, oneshot::Receiver<NodeToServer>)> {
        let _capacity = self.capacity.lock();
        if self.status_requests.len() >= MAX_PENDING_STATUS_REQUESTS {
            anyhow::bail!("hosted MCP correlation capacity reached");
        }
        let request_id = uuid::Uuid::new_v4().to_string();
        let (sender, receiver) = oneshot::channel();
        self.status_requests.insert(
            request_id.clone(),
            McpStatusPending {
                device_id: device_id.to_owned(),
                sender,
            },
        );
        Ok((request_id, receiver))
    }

    pub fn cancel_status_request(&self, request_id: &str) {
        self.status_requests.remove(request_id);
    }

    pub fn release_device(&self, device_id: &str) -> Vec<String> {
        self.release_images(device_id);
        self.status_requests
            .retain(|_, pending| pending.device_id != device_id);
        Vec::new()
    }
}
