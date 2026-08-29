mod image;
mod output;

use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use dashmap::DashMap;
use output::{McpInner, append, snapshot};
use parking_lot::Mutex;
use rc_protocol::NodeToServer;
use std::sync::Arc;
use tokio::sync::{Notify, oneshot};

const COMPLETED_TTL_MS: i64 = 5 * 60_000;
const MAX_ACTIVE_PROCESSES: usize = 128;
const MAX_COMPLETED_STATES: usize = 256;

pub use output::{McpOutputChunk, McpProcessResult};

#[derive(Clone, Default)]
pub struct McpHub {
    states: Arc<DashMap<String, Arc<McpState>>>,
    registration: Arc<Mutex<()>>,
    images: Arc<DashMap<String, Arc<McpImagePending>>>,
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

pub(super) struct McpState {
    grant_id: String,
    user_id: String,
    device_id: String,
    inner: Mutex<McpInner>,
    notify: Notify,
}

impl McpHub {
    pub fn register(
        &self,
        process_id: &str,
        grant_id: &str,
        user_id: &str,
        device_id: &str,
    ) -> anyhow::Result<()> {
        let _registration = self.registration.lock();
        self.cleanup();
        let active = self
            .states
            .iter()
            .filter(|entry| entry.inner.lock().status == "running")
            .count();
        if active >= MAX_ACTIVE_PROCESSES {
            anyhow::bail!("too many active MCP processes");
        }
        if self.states.contains_key(process_id) {
            anyhow::bail!("MCP process ID is already active");
        }
        self.states.insert(
            process_id.to_owned(),
            Arc::new(McpState {
                grant_id: grant_id.to_owned(),
                user_id: user_id.to_owned(),
                device_id: device_id.to_owned(),
                inner: Mutex::new(McpInner::new()),
                notify: Notify::new(),
            }),
        );
        Ok(())
    }

    pub fn remove(&self, process_id: &str) {
        self.states.remove(process_id);
    }

    pub fn handle(&self, device_id: &str, message: &NodeToServer) -> Option<(String, i32, String)> {
        match message {
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
            NodeToServer::McpStdout { process_id, data } => {
                self.append_message(device_id, process_id, "stdout", data);
                None
            }
            NodeToServer::McpStderr { process_id, data } => {
                self.append_message(device_id, process_id, "stderr", data);
                None
            }
            NodeToServer::McpExit {
                process_id,
                exit_code,
                signal,
            } => {
                let state = self.states.get(process_id).map(|entry| entry.clone())?;
                if state.device_id != device_id {
                    return None;
                }
                let mut inner = state.inner.lock();
                inner.status = "exited";
                inner.exit_code = Some(*exit_code);
                inner.signal = (!signal.is_empty()).then_some(signal.clone());
                inner.updated_at = crate::now_ms();
                drop(inner);
                state.notify.notify_waiters();
                Some((process_id.clone(), *exit_code, signal.clone()))
            }
            _ => None,
        }
    }

    pub fn mark_lost(&self, process_id: &str, error: impl Into<String>) {
        let Some(state) = self.states.get(process_id).map(|entry| entry.clone()) else {
            return;
        };
        let mut inner = state.inner.lock();
        if inner.status != "running" {
            return;
        }
        inner.status = "lost";
        inner.error = Some(error.into());
        inner.updated_at = crate::now_ms();
        drop(inner);
        state.notify.notify_waiters();
    }

    pub async fn result(
        &self,
        process_id: &str,
        grant_id: &str,
        user_id: &str,
        cursor: usize,
        wait_seconds: u64,
    ) -> anyhow::Result<McpProcessResult> {
        self.cleanup();
        let state = self.authorized_state(process_id, grant_id, user_id)?;
        let initial = snapshot(process_id, &state, cursor);
        if wait_seconds == 0
            || initial.status != "running"
            || !initial.chunks.is_empty()
            || initial.truncated_before_cursor > cursor
        {
            return Ok(initial);
        }
        let wait = std::time::Duration::from_secs(wait_seconds.min(60));
        let _ = tokio::time::timeout(wait, state.notify.notified()).await;
        Ok(snapshot(process_id, &state, cursor))
    }

    pub fn running_device(
        &self,
        process_id: &str,
        grant_id: &str,
        user_id: &str,
    ) -> anyhow::Result<String> {
        self.cleanup();
        let state = self.authorized_state(process_id, grant_id, user_id)?;
        if state.inner.lock().status != "running" {
            anyhow::bail!("process is no longer running");
        }
        Ok(state.device_id.clone())
    }

    pub fn release_device(&self, device_id: &str) -> Vec<String> {
        self.release_images(device_id);
        let ids: Vec<_> = self
            .states
            .iter()
            .filter(|entry| entry.device_id == device_id)
            .map(|entry| entry.key().clone())
            .collect();
        for id in &ids {
            self.mark_lost(id, "RC Node disconnected");
        }
        ids
    }

    fn authorized_state(
        &self,
        process_id: &str,
        grant_id: &str,
        user_id: &str,
    ) -> anyhow::Result<Arc<McpState>> {
        let state = self
            .states
            .get(process_id)
            .map(|entry| entry.clone())
            .ok_or_else(|| anyhow::anyhow!("process is unavailable for this MCP grant"))?;
        if state.grant_id != grant_id || state.user_id != user_id {
            anyhow::bail!("process is unavailable for this MCP grant");
        }
        Ok(state)
    }

    fn append_message(&self, device_id: &str, process_id: &str, stream: &'static str, data: &str) {
        let Some(state) = self.states.get(process_id).map(|entry| entry.clone()) else {
            return;
        };
        if state.device_id != device_id {
            return;
        }
        if let Ok(bytes) = URL_SAFE_NO_PAD.decode(data) {
            append(&state, stream, bytes);
        }
    }

    fn cleanup(&self) {
        let now = crate::now_ms();
        let mut completed = Vec::new();
        for entry in self.states.iter() {
            let inner = entry.inner.lock();
            if inner.status != "running" {
                completed.push((inner.updated_at, entry.key().clone()));
            }
        }
        completed.sort_by_key(|entry| entry.0);
        let excess = completed.len().saturating_sub(MAX_COMPLETED_STATES);
        for (index, (updated_at, id)) in completed.into_iter().enumerate() {
            if updated_at + COMPLETED_TTL_MS < now || index < excess {
                self.states.remove(&id);
            }
        }
    }
}
