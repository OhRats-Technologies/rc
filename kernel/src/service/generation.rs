use super::ActiveInstance;
use std::sync::{Arc, Mutex};

pub type InstanceHandle = Arc<Generation>;

pub struct Generation {
    state: Mutex<State>,
}

struct State {
    active: Option<ActiveInstance>,
    pins: usize,
    withdrawn: bool,
}

impl Generation {
    pub(crate) fn new(active: ActiveInstance) -> InstanceHandle {
        Arc::new(Self {
            state: Mutex::new(State {
                active: Some(active),
                pins: 0,
                withdrawn: false,
            }),
        })
    }

    pub(crate) fn pin(self: &InstanceHandle) -> Option<GenerationPin> {
        let mut state = self.state.lock().expect("component generation poisoned");
        if state.withdrawn || state.active.is_none() {
            return None;
        }
        state.pins += 1;
        Some(GenerationPin {
            generation: self.clone(),
        })
    }

    pub(crate) fn is_available(&self) -> bool {
        let state = self.state.lock().expect("component generation poisoned");
        !state.withdrawn && state.active.is_some()
    }

    pub(crate) fn invoke(
        self: &InstanceHandle,
        command: &str,
        args: &[String],
    ) -> anyhow::Result<u32> {
        let _pin = self
            .pin()
            .ok_or_else(|| anyhow::anyhow!("component generation is unavailable"))?;
        self.with_active(|active| {
            active
                .invoke(command, args)
                .map_err(|error| wasmtime::format_err!("{error:#}"))
        })
        .map_err(|error| anyhow::anyhow!("{error:#}"))
    }

    pub(crate) fn with_active<T>(
        &self,
        operation: impl FnOnce(&mut ActiveInstance) -> wasmtime::Result<T>,
    ) -> wasmtime::Result<T> {
        let mut state = self.state.lock().expect("component generation poisoned");
        let active = state
            .active
            .as_mut()
            .ok_or_else(|| wasmtime::format_err!("component generation is unavailable"))?;
        operation(active)
    }

    pub(crate) fn withdraw(&self) {
        let mut state = self.state.lock().expect("component generation poisoned");
        state.withdrawn = true;
    }

    pub(crate) fn deactivate(&self) {
        let active = {
            let mut state = self.state.lock().expect("component generation poisoned");
            state.withdrawn = true;
            take_ready(&mut state)
        };
        deactivate(active);
    }

    fn release(&self) {
        let active = {
            let mut state = self.state.lock().expect("component generation poisoned");
            state.pins = state.pins.checked_sub(1).expect("generation pin underflow");
            take_ready(&mut state)
        };
        deactivate(active);
    }
}

fn take_ready(state: &mut State) -> Option<ActiveInstance> {
    (state.withdrawn && state.pins == 0)
        .then(|| state.active.take())
        .flatten()
}

fn deactivate(mut active: Option<ActiveInstance>) {
    if let Some(active) = active.as_mut() {
        active.deactivate();
    }
}

pub(crate) struct GenerationPin {
    generation: InstanceHandle,
}

impl Drop for GenerationPin {
    fn drop(&mut self) {
        self.generation.release();
    }
}
