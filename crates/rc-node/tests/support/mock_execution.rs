use parking_lot::Mutex;
use rc_node::{
    ExecutionManager, ExecutionRead, ProcessEvent, ProcessEventSink, ProcessExecutionMode,
    ProcessRelaySink, ProcessSecureSink, ProcessSpec, StreamKind,
};
use std::{collections::HashMap, io, sync::Arc, time::Duration};

pub struct MockExecutionManager {
    lifecycle: ProcessEventSink,
    secure: Mutex<Option<ProcessSecureSink>>,
    relay: Mutex<Option<ProcessRelaySink>>,
    executions: Arc<Mutex<HashMap<String, Entry>>>,
}

#[derive(Clone)]
struct Entry {
    owner: String,
    authorization_id: String,
    channel: rc_node::ProcessChannel,
    session: String,
    relay: String,
}

impl MockExecutionManager {
    pub fn new(lifecycle: ProcessEventSink) -> Self {
        Self {
            lifecycle,
            secure: Mutex::new(None),
            relay: Mutex::new(None),
            executions: Arc::new(Mutex::new(HashMap::new())),
        }
    }

    fn dispatch(&self, entry: &Entry, event: ProcessEvent) {
        let handled = if !entry.session.is_empty() {
            self.secure
                .lock()
                .as_ref()
                .is_some_and(|sink| sink(&entry.session, event.clone()))
        } else if !entry.relay.is_empty() {
            self.relay
                .lock()
                .as_ref()
                .is_some_and(|sink| sink(&entry.relay, event.clone()))
        } else {
            false
        };
        if !handled
            || matches!(
                event,
                ProcessEvent::Started { .. } | ProcessEvent::Exit { .. }
            )
        {
            (self.lifecycle)(event);
        }
    }
}

impl ExecutionManager for MockExecutionManager {
    fn set_secure_sink(&self, sink: ProcessSecureSink) {
        *self.secure.lock() = Some(sink);
    }
    fn clear_secure_sink(&self) {
        *self.secure.lock() = None;
    }
    fn set_relay_sink(&self, sink: ProcessRelaySink) {
        *self.relay.lock() = Some(sink);
    }
    fn clear_relay_sink(&self) {
        *self.relay.lock() = None;
    }
    fn active_ids(&self) -> Vec<String> {
        self.executions.lock().keys().cloned().collect()
    }
    fn start(&self, spec: ProcessSpec) -> io::Result<bool> {
        let entry = Entry {
            owner: spec.user_id,
            authorization_id: spec.authorization_id,
            channel: spec.channel,
            session: spec.session_id,
            relay: spec.relay_id,
        };
        if self
            .executions
            .lock()
            .insert(spec.id.clone(), entry.clone())
            .is_some()
        {
            return Ok(false);
        }
        self.dispatch(
            &entry,
            ProcessEvent::Started {
                id: spec.id.clone(),
            },
        );
        let bytes = match spec.mode {
            ProcessExecutionMode::SystemShell { command } => {
                command.split_once("printf '").and_then(|(_, tail)| {
                    tail.split_once('\'')
                        .map(|(value, _)| value.as_bytes().to_vec())
                })
            }
            _ => None,
        };
        if let Some(bytes) = bytes {
            self.dispatch(
                &entry,
                ProcessEvent::output(StreamKind::Stdout, &spec.id, &bytes),
            );
        }
        std::thread::sleep(Duration::from_millis(2));
        self.executions.lock().remove(&spec.id);
        self.dispatch(
            &entry,
            ProcessEvent::Exit {
                id: spec.id,
                exit_code: 0,
                signal: String::new(),
            },
        );
        Ok(true)
    }
    fn input(&self, _: &str, _: &[u8]) -> io::Result<()> {
        Ok(())
    }
    fn close_input(&self, _: &str) {}
    fn resize(&self, _: &str, _: u16, _: u16) -> io::Result<()> {
        Ok(())
    }
    fn signal(&self, id: &str, signal: &str) -> io::Result<()> {
        if let Some(entry) = self.executions.lock().remove(id) {
            self.dispatch(
                &entry,
                ProcessEvent::Exit {
                    id: id.into(),
                    exit_code: 1,
                    signal: signal.into(),
                },
            );
        }
        Ok(())
    }
    fn owner(&self, id: &str) -> Option<String> {
        self.executions
            .lock()
            .get(id)
            .map(|value| value.owner.clone())
    }
    fn execution_authority(&self, id: &str) -> Option<(rc_node::ProcessChannel, String)> {
        self.executions
            .lock()
            .get(id)
            .map(|value| (value.channel, value.authorization_id.clone()))
    }
    fn shutdown(&self) {
        self.executions.lock().clear();
    }
    fn execution_read(&self, _: &str, _: u64, _: usize) -> Option<ExecutionRead> {
        None
    }
    fn attach_secure(&self, id: &str, session: &str) -> bool {
        self.executions.lock().get_mut(id).is_some_and(|entry| {
            entry.session = session.into();
            true
        })
    }
    fn secure_writer(&self, id: &str, session: &str) -> bool {
        self.executions
            .lock()
            .get(id)
            .is_some_and(|entry| entry.session == session)
    }
    fn detach_secure_session(&self, session: &str) {
        for entry in self
            .executions
            .lock()
            .values_mut()
            .filter(|entry| entry.session == session)
        {
            entry.session.clear();
        }
    }
    fn relay_process(&self, relay: &str) -> Option<String> {
        self.executions
            .lock()
            .iter()
            .find(|(_, entry)| entry.relay == relay)
            .map(|(id, _)| id.clone())
    }
    fn relay_process_ids(&self) -> Vec<String> {
        self.active_ids()
    }
}
