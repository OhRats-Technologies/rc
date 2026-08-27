mod events;
mod spawn;

use self::{
    events::emit_to,
    spawn::{capture_reader, exit_result, set_terminal_size, spawn},
};
use crate::process::{ProcessEvent, session::signal_session};
use nix::{
    sys::signal::{Signal, kill},
    unistd::Pid,
};
use parking_lot::Mutex;
use rc_protocol::TerminalSpec;
use std::{
    collections::{HashMap, VecDeque},
    fs::File,
    io::{self, Write},
    path::PathBuf,
    process::ChildStdin,
    sync::Arc,
    thread,
};

type EventSink = Arc<dyn Fn(ProcessEvent) + Send + Sync>;
type SecureSink = Arc<dyn Fn(&str, ProcessEvent) -> bool + Send + Sync>;
type RelaySink = Arc<dyn Fn(&str, ProcessEvent) -> bool + Send + Sync>;

#[derive(Debug, Clone)]
pub struct ProcessSpec {
    pub id: String,
    pub command: String,
    pub cwd: String,
    pub terminal: Option<TerminalSpec>,
    pub session_id: String,
    pub user_id: String,
    pub secure: bool,
    pub relay_id: String,
}

impl ProcessSpec {
    pub fn command(id: &str, command: &str) -> Self {
        Self {
            id: id.into(),
            command: command.into(),
            cwd: String::new(),
            terminal: None,
            session_id: String::new(),
            user_id: String::new(),
            secure: false,
            relay_id: String::new(),
        }
    }
}

pub struct ProcessManager {
    runner: PathBuf,
    processes: Arc<Mutex<HashMap<String, Arc<ManagedProcess>>>>,
    event_sink: EventSink,
    secure_sink: Arc<Mutex<Option<SecureSink>>>,
    relay_sink: Arc<Mutex<Option<RelaySink>>>,
}

struct ManagedProcess {
    pid: i32,
    input: Mutex<ProcessInput>,
    lifeline: Mutex<Option<File>>,
    secure: bool,
    user_id: String,
    relay_id: String,
    secure_state: Mutex<SecureState>,
}

enum ProcessInput {
    Pipe(Option<ChildStdin>),
    Pty(File),
}

#[derive(Default)]
struct SecureState {
    session_id: String,
    scrollback: VecDeque<ProcessEvent>,
    scrollback_bytes: usize,
}

impl ProcessManager {
    pub fn new(runner: PathBuf, event_sink: impl Fn(ProcessEvent) + Send + Sync + 'static) -> Self {
        Self {
            runner,
            processes: Arc::new(Mutex::new(HashMap::new())),
            event_sink: Arc::new(event_sink),
            secure_sink: Arc::new(Mutex::new(None)),
            relay_sink: Arc::new(Mutex::new(None)),
        }
    }

    pub fn set_secure_sink(
        &self,
        sink: impl Fn(&str, ProcessEvent) -> bool + Send + Sync + 'static,
    ) {
        *self.secure_sink.lock() = Some(Arc::new(sink));
    }

    pub fn clear_secure_sink(&self) {
        *self.secure_sink.lock() = None;
    }

    pub fn set_relay_sink(
        &self,
        sink: impl Fn(&str, ProcessEvent) -> bool + Send + Sync + 'static,
    ) {
        *self.relay_sink.lock() = Some(Arc::new(sink));
    }

    pub fn clear_relay_sink(&self) {
        *self.relay_sink.lock() = None;
    }

    pub fn active_ids(&self) -> Vec<String> {
        self.processes.lock().keys().cloned().collect()
    }

    pub fn start(&self, spec: ProcessSpec) -> io::Result<bool> {
        if spec.id.is_empty() || spec.command.trim().is_empty() {
            return Ok(false);
        }
        let mut processes = self.processes.lock();
        if processes.contains_key(&spec.id) {
            return Ok(false);
        }
        let (managed, mut child, readers) = spawn(&self.runner, &spec)?;
        let managed = Arc::new(managed);
        processes.insert(spec.id.clone(), managed.clone());
        drop(processes);
        self.emit(
            &managed,
            ProcessEvent::Started {
                id: spec.id.clone(),
            },
        );
        let event_sink = self.event_sink.clone();
        let secure_sink = self.secure_sink.clone();
        let relay_sink = self.relay_sink.clone();
        let captures: Vec<_> = readers
            .into_iter()
            .map(|(kind, reader)| {
                let id = spec.id.clone();
                let process = managed.clone();
                let event_sink = event_sink.clone();
                let secure_sink = secure_sink.clone();
                let relay_sink = relay_sink.clone();
                thread::spawn(move || {
                    capture_reader(
                        reader,
                        kind,
                        id,
                        process,
                        event_sink,
                        secure_sink,
                        relay_sink,
                    )
                })
            })
            .collect();
        let map = self.processes.clone();
        thread::spawn(move || {
            let status = child.wait();
            for capture in captures {
                let _ = capture.join();
            }
            map.lock().remove(&spec.id);
            managed.lifeline.lock().take();
            let (exit_code, signal) = exit_result(status);
            emit_to(
                &event_sink,
                &secure_sink,
                &relay_sink,
                &managed,
                ProcessEvent::Exit {
                    id: spec.id,
                    exit_code,
                    signal,
                },
            );
        });
        Ok(true)
    }

    pub fn input(&self, id: &str, data: &[u8]) -> io::Result<()> {
        let Some(process) = self.processes.lock().get(id).cloned() else {
            return Ok(());
        };
        match &mut *process.input.lock() {
            ProcessInput::Pipe(Some(stdin)) => stdin.write_all(data),
            ProcessInput::Pty(master) => master.write_all(data),
            ProcessInput::Pipe(None) => Ok(()),
        }
    }

    pub fn close_input(&self, id: &str) {
        if let Some(process) = self.processes.lock().get(id).cloned()
            && let ProcessInput::Pipe(stdin) = &mut *process.input.lock()
        {
            stdin.take();
        }
    }

    pub fn resize(&self, id: &str, cols: u16, rows: u16) -> io::Result<()> {
        let Some(process) = self.processes.lock().get(id).cloned() else {
            return Ok(());
        };
        if let ProcessInput::Pty(master) = &*process.input.lock() {
            set_terminal_size(master, cols, rows)?;
        }
        Ok(())
    }

    pub fn signal(&self, id: &str, signal: &str) -> io::Result<()> {
        let Some(process) = self.processes.lock().get(id).cloned() else {
            return Ok(());
        };
        match signal.to_ascii_uppercase().as_str() {
            "INT" => match &mut *process.input.lock() {
                ProcessInput::Pty(master) => master.write_all(&[3])?,
                ProcessInput::Pipe(_) => signal_session(process.pid, Signal::SIGINT),
            },
            "KILL" => {
                process.lifeline.lock().take();
            }
            _ => {
                let _ = kill(Pid::from_raw(process.pid), Signal::SIGTERM);
            }
        }
        Ok(())
    }

    pub fn attach_secure(&self, id: &str, session_id: &str) -> bool {
        let Some(process) = self.processes.lock().get(id).cloned() else {
            return false;
        };
        if !process.secure {
            return false;
        }
        let sink = self.secure_sink.lock().clone();
        let mut state = process.secure_state.lock();
        state.session_id = session_id.into();
        if let Some(sink) = sink {
            for event in state.scrollback.clone() {
                if !sink(session_id, event) {
                    state.session_id.clear();
                    return false;
                }
            }
        }
        true
    }

    pub fn detach_secure_session(&self, session_id: &str) {
        for process in self.processes.lock().values() {
            if !process.secure {
                continue;
            }
            let mut state = process.secure_state.lock();
            if state.session_id == session_id {
                state.session_id.clear();
            }
        }
    }

    pub fn owner(&self, id: &str) -> Option<String> {
        self.processes
            .lock()
            .get(id)
            .map(|process| process.user_id.clone())
    }

    pub fn shutdown(&self) {
        let values: Vec<_> = self.processes.lock().values().cloned().collect();
        for process in values {
            process.lifeline.lock().take();
        }
    }

    fn emit(&self, process: &Arc<ManagedProcess>, event: ProcessEvent) {
        emit_to(
            &self.event_sink,
            &self.secure_sink,
            &self.relay_sink,
            process,
            event,
        );
    }
}
