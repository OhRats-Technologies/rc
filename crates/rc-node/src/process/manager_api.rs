use super::{ProcessEvent, ProcessSpec, StreamKind};
use crate::ProcessChannel;
use std::{io, sync::Arc};

pub type ProcessEventSink = Arc<dyn Fn(ProcessEvent) + Send + Sync>;
pub type ProcessSecureSink = Arc<dyn Fn(&str, ProcessEvent) -> bool + Send + Sync>;
pub type ProcessRelaySink = Arc<dyn Fn(&str, ProcessEvent) -> bool + Send + Sync>;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct JournalChunk {
    pub stream: StreamKind,
    pub cursor: u64,
    pub bytes: Vec<u8>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ExecutionRead {
    pub status: &'static str,
    pub chunks: Vec<JournalChunk>,
    pub next_cursor: u64,
    pub truncated_before: u64,
    pub more: bool,
    pub exit_code: Option<i32>,
    pub signal: String,
}

pub trait ExecutionManager: Send + Sync {
    fn set_secure_sink(&self, sink: ProcessSecureSink);
    fn clear_secure_sink(&self);
    fn set_relay_sink(&self, sink: ProcessRelaySink);
    fn clear_relay_sink(&self);
    fn active_ids(&self) -> Vec<String>;
    fn start(&self, spec: ProcessSpec) -> io::Result<bool>;
    fn input(&self, id: &str, data: &[u8]) -> io::Result<()>;
    fn close_input(&self, id: &str);
    fn resize(&self, id: &str, cols: u16, rows: u16) -> io::Result<()>;
    fn signal(&self, id: &str, signal: &str) -> io::Result<()>;
    fn owner(&self, id: &str) -> Option<String>;
    fn execution_authority(&self, id: &str) -> Option<(ProcessChannel, String)>;
    fn shutdown(&self);
    fn execution_read(&self, id: &str, cursor: u64, max_bytes: usize) -> Option<ExecutionRead>;
    fn attach_secure(&self, id: &str, session_id: &str) -> bool;
    fn secure_writer(&self, id: &str, session_id: &str) -> bool;
    fn detach_secure_session(&self, session_id: &str);
    fn relay_process(&self, relay_id: &str) -> Option<String>;
    fn relay_process_ids(&self) -> Vec<String>;
}
