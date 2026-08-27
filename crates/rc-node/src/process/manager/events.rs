use super::{EventSink, ManagedProcess, RelaySink, SecureSink};
use crate::process::ProcessEvent;
use parking_lot::Mutex;
use std::sync::Arc;

const SCROLLBACK_LIMIT: usize = 4 << 20;

pub(super) fn emit_to(
    event_sink: &EventSink,
    secure_sink: &Arc<Mutex<Option<SecureSink>>>,
    relay_sink: &Arc<Mutex<Option<RelaySink>>>,
    process: &Arc<ManagedProcess>,
    event: ProcessEvent,
) {
    if !process.secure {
        event_sink(event.clone());
        if !process.relay_id.is_empty()
            && let Some(sink) = relay_sink.lock().clone()
        {
            let _ = sink(&process.relay_id, event);
        }
        return;
    }
    if matches!(
        event,
        ProcessEvent::Started { .. } | ProcessEvent::Exit { .. }
    ) {
        event_sink(event.clone());
    }
    let mut state = process.secure_state.lock();
    if event.is_output() {
        let size = event.estimated_size();
        while state.scrollback_bytes + size > SCROLLBACK_LIMIT && !state.scrollback.is_empty() {
            if let Some(old) = state.scrollback.pop_front() {
                state.scrollback_bytes =
                    state.scrollback_bytes.saturating_sub(old.estimated_size());
            }
        }
        if size <= SCROLLBACK_LIMIT {
            state.scrollback.push_back(event.clone());
            state.scrollback_bytes += size;
        }
    }
    if !state.session_id.is_empty()
        && let Some(sink) = secure_sink.lock().clone()
    {
        let session = state.session_id.clone();
        if !sink(&session, event) {
            state.session_id.clear();
        }
    }
}
