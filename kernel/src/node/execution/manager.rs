use super::{ComponentExecutionRuntime, Execution};
use parking_lot::Mutex;
use rc_node::{
    ExecutionManager, ExecutionRead, ProcessEvent, ProcessEventSink, ProcessRelaySink,
    ProcessSecureSink, ProcessSignal, ProcessSpec, ProcessStartRequest,
};
use std::{
    collections::{HashMap, HashSet},
    io,
    sync::Arc,
    thread,
    time::{Duration, Instant},
};

mod attachment;
mod capacity;
mod events;
use attachment::{attach, detach};
use events::{emit_to, poll_execution};

pub struct ComponentExecutionManager {
    runtime: ComponentExecutionRuntime,
    processes: Arc<Mutex<HashMap<String, Arc<Managed>>>>,
    seen: Mutex<HashSet<String>>,
    event_sink: ProcessEventSink,
    secure_sink: Arc<Mutex<Option<ProcessSecureSink>>>,
    relay_sink: Arc<Mutex<Option<ProcessRelaySink>>>,
}

struct Managed {
    execution: Mutex<Execution>,
    user_id: String,
    authorization_id: String,
    channel: rc_node::ProcessChannel,
    relay_id: String,
    secure: bool,
    writer: Mutex<String>,
    stdin_limit: usize,
    journal_limit: usize,
    status: Mutex<Status>,
}

#[derive(Clone)]
struct Status {
    name: &'static str,
}

const MAX_EXECUTIONS_PER_NODE_RUN: usize = 65_536;
const MAX_RETAINED_EXECUTIONS: usize = 1_024;

impl ComponentExecutionManager {
    pub fn new(runtime: ComponentExecutionRuntime, event_sink: ProcessEventSink) -> Self {
        Self {
            runtime,
            processes: Arc::new(Mutex::new(HashMap::new())),
            seen: Mutex::new(HashSet::new()),
            event_sink,
            secure_sink: Arc::new(Mutex::new(None)),
            relay_sink: Arc::new(Mutex::new(None)),
        }
    }

    fn emit(&self, process: &Managed, event: ProcessEvent) {
        emit_to(
            &self.event_sink,
            &self.secure_sink,
            &self.relay_sink,
            process,
            event,
        );
    }
}

fn cleanup(
    processes: Arc<Mutex<HashMap<String, Arc<Managed>>>>,
) -> Arc<dyn Fn(&str) + Send + Sync> {
    Arc::new(move |id| {
        let mut values = processes.lock();
        if values
            .get(id)
            .is_some_and(|value| value.status.lock().name != "running")
        {
            values.remove(id);
        }
    })
}

impl ExecutionManager for ComponentExecutionManager {
    fn set_secure_sink(&self, sink: ProcessSecureSink) {
        *self.secure_sink.lock() = Some(sink)
    }
    fn clear_secure_sink(&self) {
        *self.secure_sink.lock() = None
    }
    fn set_relay_sink(&self, sink: ProcessRelaySink) {
        *self.relay_sink.lock() = Some(sink)
    }
    fn clear_relay_sink(&self) {
        *self.relay_sink.lock() = None
    }

    fn active_ids(&self) -> Vec<String> {
        self.processes
            .lock()
            .iter()
            .filter(|(_, value)| value.status.lock().name == "running")
            .map(|(id, _)| id.clone())
            .collect()
    }

    fn start(&self, spec: ProcessSpec) -> io::Result<bool> {
        let mut processes = self.processes.lock();
        let mut seen = self.seen.lock();
        if seen.contains(&spec.id)
            || (!spec.relay_id.is_empty()
                && processes
                    .values()
                    .any(|value| value.relay_id == spec.relay_id))
        {
            return Ok(false);
        }
        let journal_limit = spec.scrollback_bytes as usize;
        let journal_bytes = processes
            .values()
            .map(|value| value.journal_limit)
            .sum::<usize>();
        if seen.len() >= MAX_EXECUTIONS_PER_NODE_RUN
            || processes.len() >= MAX_RETAINED_EXECUTIONS
            || !capacity::journal(journal_bytes, journal_limit)
        {
            return Err(io::Error::other("execution registry capacity reached"));
        }
        seen.insert(spec.id.clone());
        let request = ProcessStartRequest {
            execution_id: spec.id.clone(),
            mode: spec.mode,
            cwd: (!spec.cwd.is_empty()).then_some(spec.cwd),
            environment: spec.environment,
            terminal: spec.terminal,
            channel: spec.channel,
            lifetime: spec.lifetime,
            principal: spec.principal,
            max_runtime_ms: spec.max_runtime_ms,
        };
        let execution = self.runtime.start(request).map_err(io::Error::other)?;
        if spec.secure && !spec.session_id.is_empty() {
            execution
                .attach(&spec.session_id)
                .map_err(io::Error::other)?;
        }
        let managed = Arc::new(Managed {
            execution: Mutex::new(execution),
            user_id: spec.user_id,
            authorization_id: spec.authorization_id,
            channel: spec.channel,
            relay_id: spec.relay_id,
            secure: spec.secure,
            writer: Mutex::new(spec.session_id),
            stdin_limit: spec.stdin_chunk_bytes as usize,
            journal_limit,
            status: Mutex::new(Status { name: "running" }),
        });
        processes.insert(spec.id.clone(), managed.clone());
        drop(processes);
        self.emit(
            &managed,
            ProcessEvent::Started {
                id: spec.id.clone(),
            },
        );
        poll_execution(
            spec.id,
            managed,
            self.event_sink.clone(),
            self.secure_sink.clone(),
            self.relay_sink.clone(),
            cleanup(self.processes.clone()),
        );
        Ok(true)
    }

    fn input(&self, id: &str, data: &[u8]) -> io::Result<()> {
        let process = self
            .processes
            .lock()
            .get(id)
            .cloned()
            .ok_or_else(|| io::Error::new(io::ErrorKind::NotFound, "execution not found"))?;
        if data.len() > process.stdin_limit {
            return Err(io::Error::new(
                io::ErrorKind::InvalidInput,
                "process input exceeds policy limit",
            ));
        }
        let execution = process.execution.lock();
        let mut offset = 0;
        let deadline = Instant::now() + Duration::from_secs(30);
        while offset < data.len() {
            let count = execution.input(&data[offset..]).map_err(io::Error::other)? as usize;
            if count == 0 {
                if Instant::now() >= deadline || process.status.lock().name != "running" {
                    return Err(io::Error::new(
                        io::ErrorKind::TimedOut,
                        "process input remained blocked",
                    ));
                }
                thread::sleep(Duration::from_millis(2));
            } else {
                offset += count;
            }
        }
        Ok(())
    }

    fn close_input(&self, id: &str) {
        if let Some(value) = self.processes.lock().get(id) {
            let _ = value.execution.lock().close_input();
        }
    }
    fn resize(&self, id: &str, cols: u16, rows: u16) -> io::Result<()> {
        self.with_execution(id, |value| value.resize(cols, rows))
    }
    fn signal(&self, id: &str, signal: &str) -> io::Result<()> {
        let signal = ProcessSignal::parse(signal).map_err(io::Error::other)?;
        self.with_execution(id, |value| value.signal(signal))
    }
    fn owner(&self, id: &str) -> Option<String> {
        self.processes
            .lock()
            .get(id)
            .map(|value| value.user_id.clone())
    }
    fn execution_authority(&self, id: &str) -> Option<(rc_node::ProcessChannel, String)> {
        self.processes
            .lock()
            .get(id)
            .map(|value| (value.channel, value.authorization_id.clone()))
    }
    fn shutdown(&self) {
        for value in self.processes.lock().values() {
            let _ = value.execution.lock().signal(ProcessSignal::Kill);
        }
    }
    fn execution_read(&self, id: &str, cursor: u64, max: usize) -> Option<ExecutionRead> {
        let value = self.processes.lock().get(id).cloned()?;
        value
            .execution
            .lock()
            .read(cursor, max.min(u32::MAX as usize) as u32)
            .ok()
    }
    fn attach_secure(&self, id: &str, session: &str) -> bool {
        attach(self, id, session)
    }
    fn secure_writer(&self, id: &str, session: &str) -> bool {
        self.processes
            .lock()
            .get(id)
            .is_some_and(|value| value.secure && *value.writer.lock() == session)
    }
    fn detach_secure_session(&self, session: &str) {
        detach(self, session)
    }
    fn relay_process(&self, relay: &str) -> Option<String> {
        self.processes
            .lock()
            .iter()
            .find_map(|(id, value)| (value.relay_id == relay).then(|| id.clone()))
    }
    fn relay_process_ids(&self) -> Vec<String> {
        self.processes
            .lock()
            .iter()
            .filter(|(_, value)| !value.relay_id.is_empty())
            .map(|(id, _)| id.clone())
            .collect()
    }
}

impl ComponentExecutionManager {
    fn with_execution(
        &self,
        id: &str,
        call: impl FnOnce(&Execution) -> Result<(), String>,
    ) -> io::Result<()> {
        let value = self
            .processes
            .lock()
            .get(id)
            .cloned()
            .ok_or_else(|| io::Error::new(io::ErrorKind::NotFound, "execution not found"))?;
        call(&value.execution.lock()).map_err(io::Error::other)
    }
}
