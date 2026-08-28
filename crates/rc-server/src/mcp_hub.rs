use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use dashmap::DashMap;
use parking_lot::Mutex;
use rc_protocol::NodeToServer;
use std::{collections::VecDeque, sync::Arc};
use tokio::sync::Notify;

const OUTPUT_LIMIT: usize = 256 * 1024;

#[derive(Debug, Clone, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct McpProcessResult {
    pub process_id: String,
    pub status: String,
    pub output: String,
    pub exit_code: Option<i32>,
    pub signal: Option<String>,
    pub error: Option<String>,
    pub next_offset: usize,
    pub output_truncated: bool,
}

#[derive(Clone, Default)]
pub struct McpHub {
    states: Arc<DashMap<String, Arc<McpState>>>,
}

struct McpState {
    grant_id: String,
    user_id: String,
    device_id: String,
    inner: Mutex<McpInner>,
    notify: Notify,
}

struct McpInner {
    status: &'static str,
    output: VecDeque<u8>,
    total: usize,
    truncated: bool,
    exit_code: Option<i32>,
    signal: Option<String>,
    error: Option<String>,
    updated_at: i64,
}

impl McpHub {
    pub fn register(&self, process_id: &str, grant_id: &str, user_id: &str, device_id: &str) {
        self.cleanup();
        self.states.insert(
            process_id.to_owned(),
            Arc::new(McpState {
                grant_id: grant_id.to_owned(),
                user_id: user_id.to_owned(),
                device_id: device_id.to_owned(),
                inner: Mutex::new(McpInner {
                    status: "running",
                    output: VecDeque::new(),
                    total: 0,
                    truncated: false,
                    exit_code: None,
                    signal: None,
                    error: None,
                    updated_at: crate::now_ms(),
                }),
                notify: Notify::new(),
            }),
        );
    }

    pub fn handle(&self, device_id: &str, message: &NodeToServer) -> Option<(String, i32, String)> {
        match message {
            NodeToServer::McpStdout { process_id, data }
            | NodeToServer::McpStderr { process_id, data } => {
                let state = self.states.get(process_id).map(|entry| entry.clone())?;
                if state.device_id != device_id {
                    return None;
                }
                if let Ok(bytes) = URL_SAFE_NO_PAD.decode(data) {
                    append(&state, &bytes);
                }
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
        inner.error = Some(error.into().chars().take(1024).collect());
        inner.updated_at = crate::now_ms();
        drop(inner);
        state.notify.notify_waiters();
    }

    pub async fn result(
        &self,
        process_id: &str,
        grant_id: &str,
        user_id: &str,
        offset: usize,
        wait_seconds: u64,
    ) -> anyhow::Result<McpProcessResult> {
        let state = self
            .states
            .get(process_id)
            .map(|entry| entry.clone())
            .ok_or_else(|| anyhow::anyhow!("process status is unavailable for this MCP grant"))?;
        if state.grant_id != grant_id || state.user_id != user_id {
            anyhow::bail!("process status is unavailable for this MCP grant");
        }
        let initial = snapshot(process_id, &state, offset);
        if wait_seconds == 0 || initial.status != "running" || initial.next_offset > offset {
            return Ok(initial);
        }
        let wait = std::time::Duration::from_secs(wait_seconds.min(60));
        let _ = tokio::time::timeout(wait, state.notify.notified()).await;
        Ok(snapshot(process_id, &state, offset))
    }

    pub fn running_device(
        &self,
        process_id: &str,
        grant_id: &str,
        user_id: &str,
    ) -> anyhow::Result<String> {
        let state = self
            .states
            .get(process_id)
            .map(|entry| entry.clone())
            .ok_or_else(|| anyhow::anyhow!("process is unavailable for this MCP grant"))?;
        if state.grant_id != grant_id || state.user_id != user_id {
            anyhow::bail!("process is unavailable for this MCP grant");
        }
        if state.inner.lock().status != "running" {
            anyhow::bail!("process is no longer running");
        }
        Ok(state.device_id.clone())
    }

    pub fn release_device(&self, device_id: &str) -> Vec<String> {
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

    fn cleanup(&self) {
        let now = crate::now_ms();
        let mut ids: Vec<_> = self
            .states
            .iter()
            .filter_map(|entry| {
                let inner = entry.inner.lock();
                let ttl = if inner.status == "running" {
                    30 * 60_000
                } else {
                    5 * 60_000
                };
                (inner.updated_at + ttl < now).then(|| entry.key().clone())
            })
            .collect();
        if self.states.len().saturating_sub(ids.len()) >= 128 {
            let mut extra: Vec<_> = self
                .states
                .iter()
                .map(|entry| (entry.inner.lock().updated_at, entry.key().clone()))
                .collect();
            extra.sort_by_key(|value| value.0);
            ids.extend(
                extra
                    .into_iter()
                    .take(self.states.len().saturating_sub(127))
                    .map(|value| value.1),
            );
        }
        ids.sort();
        ids.dedup();
        for id in ids {
            self.states.remove(&id);
        }
    }
}

fn append(state: &McpState, bytes: &[u8]) {
    let mut inner = state.inner.lock();
    inner.total = inner.total.saturating_add(bytes.len());
    let room = OUTPUT_LIMIT.saturating_sub(inner.output.len());
    inner.output.extend(bytes.iter().copied().take(room));
    if bytes.len() > room {
        inner.truncated = true;
    }
    inner.updated_at = crate::now_ms();
    drop(inner);
    state.notify.notify_waiters();
}

fn snapshot(process_id: &str, state: &McpState, offset: usize) -> McpProcessResult {
    let inner = state.inner.lock();
    let bytes: Vec<_> = inner.output.iter().copied().collect();
    let start = offset.min(bytes.len());
    McpProcessResult {
        process_id: process_id.to_owned(),
        status: inner.status.into(),
        output: String::from_utf8_lossy(&bytes[start..]).into_owned(),
        exit_code: inner.exit_code,
        signal: inner.signal.clone(),
        error: inner.error.clone(),
        next_offset: bytes.len(),
        output_truncated: inner.truncated,
    }
}
