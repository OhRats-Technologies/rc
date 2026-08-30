mod environment;
mod filesystem;
mod process;

#[cfg(windows)]
pub(crate) fn maybe_run_windows_execution_guard() -> Option<anyhow::Result<()>> {
    process::windows::guard::maybe_run()
}

use crate::{
    bindings::ohrats::rc_process::clock_host::{Host, HostTimer, Timer},
    host::HostState,
};
use std::{collections::BTreeMap, time::Duration};
use wasmtime::component::Resource;

struct TimerValue {
    deadline_ms: u64,
    cancelled: bool,
}

#[derive(Default)]
pub(crate) struct RuntimeHandles {
    next: u32,
    timers: BTreeMap<u32, TimerValue>,
    process: process::ProcessHandles,
}

impl RuntimeHandles {
    fn insert_timer(&mut self, deadline_ms: u64) -> Result<Resource<Timer>, String> {
        self.next = self
            .next
            .checked_add(1)
            .ok_or_else(|| "timer handle space exhausted".to_owned())?;
        self.timers.insert(
            self.next,
            TimerValue {
                deadline_ms,
                cancelled: false,
            },
        );
        Ok(Resource::new_own(self.next))
    }
}

impl Host for HostState {
    fn now_unix_ms(&mut self) -> u64 {
        now_ms()
    }

    fn arm(&mut self, deadline_unix_ms: u64) -> Result<Resource<Timer>, String> {
        self.require_runtime_capability("clock")?;
        self.runtime_handles.insert_timer(deadline_unix_ms)
    }
}

impl HostTimer for HostState {
    fn wait(&mut self, timer: Resource<Timer>) -> Result<(), String> {
        self.require_runtime_capability("clock")?;
        let value = self
            .runtime_handles
            .timers
            .get(&timer.rep())
            .ok_or_else(|| "unknown timer handle".to_owned())?;
        if value.cancelled {
            return Err("timer cancelled".into());
        }
        let delay = value.deadline_ms.saturating_sub(now_ms());
        std::thread::sleep(Duration::from_millis(delay));
        Ok(())
    }

    fn cancel(&mut self, timer: Resource<Timer>) {
        if let Some(value) = self.runtime_handles.timers.get_mut(&timer.rep()) {
            value.cancelled = true;
        }
    }

    fn drop(&mut self, timer: Resource<Timer>) -> wasmtime::Result<()> {
        self.runtime_handles
            .timers
            .remove(&timer.rep())
            .ok_or_else(|| wasmtime::Error::msg("unknown timer handle"))?;
        Ok(())
    }
}

impl HostState {
    pub(crate) fn require_runtime_capability(&self, capability: &str) -> Result<(), String> {
        let allowed = match capability {
            "filesystem" | "environment" => matches!(self.plugin_id(), "ohrats:shell"),
            "clock" => matches!(
                self.plugin_id(),
                "ohrats:shell" | "ohrats:execution-runtime" | "ohrats:scheduler"
            ),
            _ => false,
        };
        allowed
            .then_some(())
            .ok_or_else(|| format!("component is not granted {capability}-host"))
    }
}

fn now_ms() -> u64 {
    use std::time::{SystemTime, UNIX_EPOCH};
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis()
        .try_into()
        .unwrap_or(u64::MAX)
}

#[cfg(test)]
mod tests {
    use super::now_ms;

    #[test]
    fn clock_is_unix_milliseconds() {
        assert!(now_ms() > 1_700_000_000_000);
    }
}
