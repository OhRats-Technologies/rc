use super::McpState;
use std::collections::VecDeque;

const OUTPUT_BUFFER_LIMIT: usize = 256 * 1024;
const OUTPUT_RESPONSE_LIMIT: usize = 64 * 1024;

#[derive(Debug, Clone, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct McpOutputChunk {
    pub stream: &'static str,
    pub text: String,
}

#[derive(Debug, Clone, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct McpProcessResult {
    pub process_id: String,
    pub status: String,
    pub chunks: Vec<McpOutputChunk>,
    pub exit_code: Option<i32>,
    pub signal: Option<String>,
    pub error: Option<String>,
    pub next_cursor: usize,
    pub output_pending: bool,
    pub truncated_before_cursor: usize,
}

pub(super) struct McpInner {
    pub(super) status: &'static str,
    output: VecDeque<BufferedChunk>,
    buffered_bytes: usize,
    total_bytes: usize,
    pub(super) exit_code: Option<i32>,
    pub(super) signal: Option<String>,
    pub(super) error: Option<String>,
    pub(super) updated_at: i64,
}

struct BufferedChunk {
    stream: &'static str,
    start: usize,
    data: Vec<u8>,
}

impl McpInner {
    pub(super) fn new() -> Self {
        Self {
            status: "running",
            output: VecDeque::new(),
            buffered_bytes: 0,
            total_bytes: 0,
            exit_code: None,
            signal: None,
            error: None,
            updated_at: crate::now_ms(),
        }
    }
}

pub(super) fn append(state: &McpState, stream: &'static str, data: Vec<u8>) {
    if data.is_empty() {
        return;
    }
    let mut inner = state.inner.lock();
    let start = inner.total_bytes;
    inner.total_bytes = inner.total_bytes.saturating_add(data.len());
    inner.buffered_bytes = inner.buffered_bytes.saturating_add(data.len());
    inner.output.push_back(BufferedChunk {
        stream,
        start,
        data,
    });
    while inner.buffered_bytes > OUTPUT_BUFFER_LIMIT {
        let excess = inner.buffered_bytes - OUTPUT_BUFFER_LIMIT;
        let Some(front) = inner.output.front_mut() else {
            break;
        };
        if front.data.len() <= excess {
            inner.buffered_bytes -= front.data.len();
            inner.output.pop_front();
        } else {
            front.data.drain(..excess);
            front.start += excess;
            inner.buffered_bytes -= excess;
        }
    }
    inner.updated_at = crate::now_ms();
    drop(inner);
    state.notify.notify_waiters();
}

pub(super) fn snapshot(process_id: &str, state: &McpState, cursor: usize) -> McpProcessResult {
    let inner = state.inner.lock();
    let earliest = inner
        .output
        .front()
        .map_or(inner.total_bytes, |chunk| chunk.start);
    let mut position = cursor.min(inner.total_bytes).max(earliest);
    let mut remaining = OUTPUT_RESPONSE_LIMIT;
    let mut chunks = Vec::new();
    for chunk in &inner.output {
        let end = chunk.start + chunk.data.len();
        if end <= position {
            continue;
        }
        let offset = position.saturating_sub(chunk.start);
        let available = &chunk.data[offset..];
        if available.len() > remaining && !chunks.is_empty() {
            break;
        }
        let take = available.len().min(remaining);
        if take == 0 {
            break;
        }
        chunks.push(McpOutputChunk {
            stream: chunk.stream,
            text: String::from_utf8_lossy(&available[..take]).into_owned(),
        });
        position += take;
        remaining -= take;
        if take < available.len() {
            break;
        }
    }
    McpProcessResult {
        process_id: process_id.to_owned(),
        status: inner.status.into(),
        chunks,
        exit_code: inner.exit_code,
        signal: inner.signal.clone(),
        error: inner.error.clone(),
        next_cursor: position,
        output_pending: position < inner.total_bytes,
        truncated_before_cursor: earliest,
    }
}
