use super::{McpHub, McpImage, McpImagePending};
use crate::NodeHub;
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use parking_lot::Mutex;
use rc_protocol::{MCP_IMAGE_MAX_BYTES, ServerToNode};
use std::{sync::Arc, time::Duration};

const IMAGE_TIMEOUT: Duration = Duration::from_secs(20);

impl McpHub {
    pub async fn request_image(
        &self,
        nodes: &NodeHub,
        device_id: &str,
        request_id: &str,
        message: &ServerToNode,
    ) -> anyhow::Result<McpImage> {
        let (tx, rx) = tokio::sync::oneshot::channel();
        let pending = Arc::new(McpImagePending {
            device_id: device_id.to_owned(),
            bytes: Mutex::new(Vec::new()),
            sender: Mutex::new(Some(tx)),
        });
        if self.images.insert(request_id.to_owned(), pending).is_some() {
            anyhow::bail!("image request ID is already active");
        }
        if let Err(error) = nodes.send(device_id, message).await {
            self.images.remove(request_id);
            anyhow::bail!("RC image request delivery failed: {error}");
        }
        match tokio::time::timeout(IMAGE_TIMEOUT, rx).await {
            Ok(Ok(Ok(image))) => Ok(image),
            Ok(Ok(Err(error))) => anyhow::bail!(error),
            Ok(Err(_)) => anyhow::bail!("RC image request was interrupted"),
            Err(_) => {
                self.images.remove(request_id);
                anyhow::bail!("RC image request timed out")
            }
        }
    }

    pub(super) fn image_chunk(&self, device_id: &str, request_id: &str, data: &str) {
        let Some(pending) = self.image_pending(device_id, request_id) else {
            return;
        };
        let Ok(chunk) = URL_SAFE_NO_PAD.decode(data) else {
            self.image_fail(request_id, pending, "invalid image transfer encoding");
            return;
        };
        let mut bytes = pending.bytes.lock();
        if bytes.len().saturating_add(chunk.len()) > MCP_IMAGE_MAX_BYTES {
            drop(bytes);
            self.image_fail(
                request_id,
                pending,
                "image transfer exceeds the configured limit",
            );
            return;
        }
        bytes.extend_from_slice(&chunk);
    }

    pub(super) fn image_finish(
        &self,
        device_id: &str,
        request_id: &str,
        mime_type: &str,
        size_bytes: u64,
        error: &str,
    ) {
        let Some(pending) = self.image_pending(device_id, request_id) else {
            return;
        };
        if !error.is_empty() {
            self.image_fail(request_id, pending, error);
            return;
        }
        if !matches!(
            mime_type,
            "image/png" | "image/jpeg" | "image/webp" | "image/gif"
        ) {
            self.image_fail(request_id, pending, "unsupported image type");
            return;
        }
        let bytes = pending.bytes.lock().clone();
        if bytes.len() as u64 != size_bytes || bytes.len() > MCP_IMAGE_MAX_BYTES {
            self.image_fail(request_id, pending, "incomplete image transfer");
            return;
        }
        self.images.remove(request_id);
        if let Some(sender) = pending.sender.lock().take() {
            let _ = sender.send(Ok(McpImage {
                mime_type: mime_type.to_owned(),
                bytes,
            }));
        }
    }

    pub(super) fn release_images(&self, device_id: &str) {
        let requests: Vec<_> = self
            .images
            .iter()
            .filter(|entry| entry.device_id == device_id)
            .map(|entry| entry.key().clone())
            .collect();
        for request_id in requests {
            if let Some((_, pending)) = self.images.remove(&request_id)
                && let Some(sender) = pending.sender.lock().take()
            {
                let _ = sender.send(Err("RC Node disconnected".into()));
            }
        }
    }

    fn image_pending(&self, device_id: &str, request_id: &str) -> Option<Arc<McpImagePending>> {
        self.images
            .get(request_id)
            .filter(|pending| pending.device_id == device_id)
            .map(|pending| pending.clone())
    }

    fn image_fail(
        &self,
        request_id: &str,
        pending: Arc<McpImagePending>,
        error: impl Into<String>,
    ) {
        self.images.remove(request_id);
        if let Some(sender) = pending.sender.lock().take() {
            let _ = sender.send(Err(error.into()));
        }
    }
}
