use crate::exports::ohrats::rc_process::runtime::{
    Execution, ExecutionState, ExitResult, Guest as RuntimeGuest, GuestExecution, ReadResult,
};
use crate::ohrats::{
    rc_plugin::types::{Requirement, Selection, Service},
    rc_process::{
        clock_host, policy,
        types::{Lifetime, Signal, StartRequest, TerminalSize},
    },
};
use crate::{
    Descriptor, Guest,
    leases::{Kind as LeaseKind, Leases},
};
use crate::{journal::Journal, native::Native};
use std::cell::RefCell;

thread_local! {
    static REGISTRY: RefCell<crate::registry::Registry> = RefCell::new(Default::default());
}

pub struct ExecutionRuntime;

pub struct RuntimeExecution(RefCell<State>);

struct State {
    id: String,
    owner_user_id: String,
    principal: crate::ohrats::rc_process::types::Principal,
    native: Native,
    journal: Journal,
    exit: Option<ExitResult>,
    leases: Leases,
    lease_kind: LeaseKind,
    registered: bool,
}

impl Guest for ExecutionRuntime {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:execution-runtime".into(),
            version: "0.1.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-process/runtime".into(),
                version: "0.3.0".into(),
                priority: 100,
                keys: Vec::new(),
            }],
            requires: vec![
                Requirement {
                    name: "ohrats:rc-process/policy".into(),
                    version: "^0.3".into(),
                    selection: Selection::Single,
                },
                Requirement {
                    name: "ohrats:rc-shell/executor".into(),
                    version: "^0.1".into(),
                    selection: Selection::Single,
                },
                Requirement {
                    name: "ohrats:rc-diagnostics/reporting".into(),
                    version: "^0.1".into(),
                    selection: Selection::Single,
                },
            ],
            commands: Vec::new(),
        }
    }

    fn activate() -> Result<(), String> {
        crate::diagnostics::activate()
    }
    fn deactivate() {}
    fn invoke(command: String, _args: Vec<String>) -> Result<u32, String> {
        Err(format!("unsupported command {command:?}"))
    }
}

impl RuntimeGuest for ExecutionRuntime {
    type Execution = RuntimeExecution;

    fn start(request: StartRequest) -> Result<Execution, String> {
        let id = request.execution_id.clone();
        let owner_user_id = request.principal.user_id.clone();
        let principal = request.principal.clone();
        let lifetime = request.lifetime;
        let plan = policy::authorize_start(&request)?;
        REGISTRY.with(|registry| registry.borrow_mut().claim(&id))?;
        crate::replay::claim(&id)?;
        let now = clock_host::now_unix_ms();
        let journal_limit = plan.scrollback_bytes;
        let lease_kind = match lifetime {
            Lifetime::Attached => LeaseKind::Attached,
            Lifetime::Managed => LeaseKind::Managed,
            Lifetime::Scheduled => LeaseKind::Scheduled,
        };
        let leases = Leases::new(
            now,
            lease_kind,
            plan.reattach_grace_ms,
            plan.terminate_grace_ms,
            plan.max_runtime_ms,
        );
        let native = Native::start(&id, plan)?;
        let counts = REGISTRY.with(|registry| registry.borrow_mut().started(lease_kind));
        crate::diagnostics::counts(counts);
        Ok(Execution::new(RuntimeExecution(RefCell::new(State {
            id,
            owner_user_id,
            principal,
            native,
            journal: Journal::new(journal_limit),
            exit: None,
            leases,
            lease_kind,
            registered: true,
        }))))
    }
}

impl GuestExecution for RuntimeExecution {
    fn id(&self) -> String {
        self.0.borrow().id.clone()
    }

    fn state(&self) -> ExecutionState {
        let mut state = self.0.borrow_mut();
        enforce_leases(&mut state);
        poll_exit(&mut state);
        if state.exit.is_some() {
            ExecutionState::Exited
        } else {
            ExecutionState::Running
        }
    }

    fn read(&self, cursor: u64, max_bytes: u32) -> Result<ReadResult, String> {
        let mut state = self.0.borrow_mut();
        enforce_leases(&mut state);
        drain(&mut state, max_bytes.max(1))?;
        poll_exit(&mut state);
        let (chunks, next_cursor, more) = state.journal.read(cursor, max_bytes as usize);
        Ok(ReadResult {
            state: if state.exit.is_some() {
                ExecutionState::Exited
            } else {
                ExecutionState::Running
            },
            chunks,
            next_cursor,
            truncated_before: state.journal.truncated(),
            more,
            exit: state.exit,
        })
    }

    fn input(&self, bytes: Vec<u8>) -> Result<u32, String> {
        let state = self.0.borrow();
        authorize_access(&state, crate::ohrats::rc_process::types::Action::Input)?;
        state.native.input(&bytes)
    }

    fn close_input(&self) -> Result<(), String> {
        let mut state = self.0.borrow_mut();
        authorize_access(&state, crate::ohrats::rc_process::types::Action::CloseInput)?;
        state.native.close_input()
    }

    fn attach(&self, controller_id: String) -> Result<(), String> {
        if controller_id.is_empty() {
            return Err("controller id is empty".into());
        }
        let mut state = self.0.borrow_mut();
        authorize_access(&state, crate::ohrats::rc_process::types::Action::Attach)?;
        state.leases.attach(controller_id)
    }

    fn detach(&self, controller_id: String) -> Result<(), String> {
        let mut state = self.0.borrow_mut();
        authorize_access(&state, crate::ohrats::rc_process::types::Action::Attach)?;
        state
            .leases
            .detach(&controller_id, clock_host::now_unix_ms());
        Ok(())
    }

    fn resize(&self, size: TerminalSize) -> Result<(), String> {
        let state = self.0.borrow();
        let normalized =
            policy::normalize_resize(&crate::ohrats::rc_process::types::ResizeRequest {
                access: access_request(&state, crate::ohrats::rc_process::types::Action::Resize),
                cols: size.cols,
                rows: size.rows,
            })?;
        state.native.resize(normalized.cols, normalized.rows)
    }

    fn signal(&self, signal: Signal) -> Result<(), String> {
        let mut state = self.0.borrow_mut();
        let signal = policy::authorize_signal(&crate::ohrats::rc_process::types::SignalRequest {
            access: access_request(&state, crate::ohrats::rc_process::types::Action::Signal),
            signal,
        })?;
        if matches!(signal, Signal::Terminate) {
            state.leases.terminate(clock_host::now_unix_ms());
        }
        state.native.signal(signal)
    }

    fn close(&self) {
        let mut state = self.0.borrow_mut();
        state.native.close();
    }
}

fn access_request(
    state: &State,
    action: crate::ohrats::rc_process::types::Action,
) -> crate::ohrats::rc_process::types::AccessRequest {
    crate::ohrats::rc_process::types::AccessRequest {
        execution_id: state.id.clone(),
        owner_user_id: state.owner_user_id.clone(),
        action,
        principal: state.principal.clone(),
    }
}

fn authorize_access(
    state: &State,
    action: crate::ohrats::rc_process::types::Action,
) -> Result<(), String> {
    policy::authorize_access(&access_request(state, action))
}

fn drain(state: &mut State, budget: u32) -> Result<(), String> {
    let (output, exit) = state.native.poll(budget)?;
    for (kind, bytes) in output {
        state.journal.push(kind, bytes);
    }
    if state.exit.is_none() && exit.is_some() {
        state.exit = exit;
        finish_registration(state);
    }
    Ok(())
}

fn poll_exit(state: &mut State) {
    if state.exit.is_some() {
        return;
    }
    if let Ok((output, exit)) = state.native.poll(1) {
        for (kind, bytes) in output {
            state.journal.push(kind, bytes);
        }
        if exit.is_some() {
            state.exit = exit;
            finish_registration(state);
        }
    }
}

fn finish_registration(state: &mut State) {
    if !state.registered {
        return;
    }
    state.registered = false;
    let counts = REGISTRY.with(|registry| registry.borrow_mut().finished(state.lease_kind));
    crate::diagnostics::counts(counts);
}

impl Drop for RuntimeExecution {
    fn drop(&mut self) {
        finish_registration(self.0.get_mut());
    }
}

fn enforce_leases(state: &mut State) {
    let now = clock_host::now_unix_ms();
    if state.leases.expired(now) {
        let _ = state.native.signal(Signal::Kill);
    }
}
