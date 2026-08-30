use super::{Managed, ProcessEventSink, ProcessRelaySink, ProcessSecureSink, Status};
use parking_lot::Mutex;
use rc_node::ProcessEvent;
use std::{sync::Arc, thread, time::Duration};

const COMPLETED_RETENTION: Duration = Duration::from_secs(5 * 60);

pub(super) fn poll_execution(
    id: String,
    process: Arc<Managed>,
    event_sink: ProcessEventSink,
    secure_sink: Arc<Mutex<Option<ProcessSecureSink>>>,
    relay_sink: Arc<Mutex<Option<ProcessRelaySink>>>,
    cleanup: Arc<dyn Fn(&str) + Send + Sync>,
) {
    thread::spawn(move || {
        let mut cursor = 0;
        loop {
            let read = process.execution.lock().read(cursor, 64 * 1024);
            let read = match read {
                Ok(value) => value,
                Err(_) => {
                    finish(
                        &id,
                        &process,
                        &event_sink,
                        &secure_sink,
                        &relay_sink,
                        -1,
                        "LOST",
                    );
                    break;
                }
            };
            for chunk in read.chunks {
                cursor = chunk.cursor.saturating_add(chunk.bytes.len() as u64);
                emit_to(
                    &event_sink,
                    &secure_sink,
                    &relay_sink,
                    &process,
                    ProcessEvent::output(chunk.stream, &id, &chunk.bytes),
                );
            }
            if read.status == "exited" || read.status == "lost" {
                finish(
                    &id,
                    &process,
                    &event_sink,
                    &secure_sink,
                    &relay_sink,
                    read.exit_code.unwrap_or(-1),
                    &read.signal,
                );
                break;
            }
            thread::sleep(Duration::from_millis(10));
        }
        thread::sleep(COMPLETED_RETENTION);
        cleanup(&id);
    });
}

fn finish(
    id: &str,
    process: &Managed,
    event_sink: &ProcessEventSink,
    secure_sink: &Arc<Mutex<Option<ProcessSecureSink>>>,
    relay_sink: &Arc<Mutex<Option<ProcessRelaySink>>>,
    exit_code: i32,
    signal: &str,
) {
    *process.status.lock() = Status {
        name: if signal == "LOST" { "lost" } else { "exited" },
    };
    emit_to(
        event_sink,
        secure_sink,
        relay_sink,
        process,
        ProcessEvent::Exit {
            id: id.into(),
            exit_code,
            signal: signal.into(),
        },
    );
}

pub(super) fn emit_to(
    event_sink: &ProcessEventSink,
    secure_sink: &Arc<Mutex<Option<ProcessSecureSink>>>,
    relay_sink: &Arc<Mutex<Option<ProcessRelaySink>>>,
    process: &Managed,
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
    let writer = process.writer.lock().clone();
    if !writer.is_empty()
        && let Some(sink) = secure_sink.lock().clone()
        && !sink(&writer, event)
    {
        process.writer.lock().clear();
    }
}
