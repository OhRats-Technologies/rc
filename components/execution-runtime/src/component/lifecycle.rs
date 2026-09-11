use super::*;
pub(super) fn drain(state: &mut State, budget: u32) -> Result<(), String> {
    if state.exit.is_some() {
        return Ok(());
    }
    let (output, exit) = match state.native.poll(budget) {
        Ok(result) => result,
        Err(error) => {
            // Execution diagnostics stay in the bounded in-memory output journal.
            state.journal.push(
                crate::ohrats::rc_process::types::StreamKind::Stderr,
                format!("RC execution failed: {error}\n").into_bytes(),
            );
            state.exit = Some(ExitResult {
                code: Some(1),
                signal: None,
            });
            finish_registration(state);
            return Ok(());
        }
    };
    for (kind, bytes) in output {
        state.journal.push(kind, bytes);
    }
    if state.exit.is_none() && exit.is_some() {
        state.exit = exit;
        finish_registration(state);
    }
    Ok(())
}

pub(super) fn poll_exit(state: &mut State) {
    if state.exit.is_some() {
        return;
    }
    let _ = drain(state, 1);
}

pub(super) fn finish_registration(state: &mut State) {
    if !state.registered {
        return;
    }
    state.registered = false;
    state.native.close();
    let counts = REGISTRY.with(|registry| registry.borrow_mut().finished(state.lease_kind));
    crate::diagnostics::counts(counts);
}

impl Drop for RuntimeExecution {
    fn drop(&mut self) {
        self.0.get_mut().native.close();
        finish_registration(self.0.get_mut());
    }
}

pub(super) fn enforce_leases(state: &mut State) {
    let now = clock_host::now_unix_ms();
    if state.leases.expired(now) {
        let _ = state.native.signal(Signal::Kill);
    }
}
