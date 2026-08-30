mod streams;
#[cfg(unix)]
mod unix;
#[cfg(windows)]
pub(crate) mod windows;
use crate::{
    bindings::ohrats::rc_process::{
        process_host::{
            ByteStream, Child, ExecutionGroup, Host, HostChild, HostExecutionGroup, NativeExit,
            SpawnRequest, Spawned, State,
        },
        types::Signal,
    },
    host::HostState,
};
use std::{
    collections::BTreeMap,
    io::{Read, Write},
};
use wasmtime::component::Resource;

struct ChildRef {
    group: u32,
    native: u32,
}

pub(super) enum StreamValue {
    Reader(Box<dyn Read + Send>),
    Writer(Box<dyn Write + Send>),
    #[cfg(unix)]
    Duplex(std::fs::File),
}

#[derive(Default)]
pub(super) struct ProcessHandles {
    next: u32,
    #[cfg(unix)]
    groups: BTreeMap<u32, unix::Group>,
    #[cfg(windows)]
    groups: BTreeMap<u32, windows::Group>,
    children: BTreeMap<u32, ChildRef>,
    streams: BTreeMap<u32, StreamValue>,
}

impl ProcessHandles {
    fn next(&mut self) -> Result<u32, String> {
        self.next = self
            .next
            .checked_add(1)
            .ok_or_else(|| "process handle space exhausted".to_owned())?;
        Ok(self.next)
    }

    #[cfg(unix)]
    fn insert_group(&mut self) -> Result<Resource<ExecutionGroup>, String> {
        let rep = self.next()?;
        self.groups.insert(rep, unix::Group::default());
        Ok(Resource::new_own(rep))
    }

    #[cfg(windows)]
    fn insert_group(&mut self) -> Result<Resource<ExecutionGroup>, String> {
        let rep = self.next()?;
        self.groups.insert(rep, windows::Group::new()?);
        Ok(Resource::new_own(rep))
    }

    fn insert_child(&mut self, group: u32, native: u32) -> Result<Resource<Child>, String> {
        let rep = self.next()?;
        self.children.insert(rep, ChildRef { group, native });
        Ok(Resource::new_own(rep))
    }

    fn insert_stream(&mut self, value: StreamValue) -> Result<Resource<ByteStream>, String> {
        let rep = self.next()?;
        self.streams.insert(rep, value);
        Ok(Resource::new_own(rep))
    }
}

impl Host for HostState {
    fn create_group(&mut self, execution_id: String) -> Result<Resource<ExecutionGroup>, String> {
        self.require_process_capability()?;
        if execution_id.is_empty() || execution_id.contains('\0') {
            return Err("invalid execution id".into());
        }
        #[cfg(unix)]
        return self.runtime_handles.process.insert_group();
        #[cfg(windows)]
        return self.runtime_handles.process.insert_group();
        #[cfg(not(any(unix, windows)))]
        Err("process-host is unavailable on this platform".into())
    }
}

impl HostExecutionGroup for HostState {
    #[allow(clippy::needless_return)]
    fn spawn(
        &mut self,
        group: Resource<ExecutionGroup>,
        request: SpawnRequest,
    ) -> Result<Spawned, String> {
        self.require_process_capability()?;
        #[cfg(unix)]
        {
            let spawned = unix::spawn(
                self.runtime_handles
                    .process
                    .groups
                    .get_mut(&group.rep())
                    .ok_or_else(|| "unknown execution group".to_owned())?,
                request,
            )?;
            let child = self
                .runtime_handles
                .process
                .insert_child(group.rep(), spawned.native_child)?;
            let stdin = spawned
                .stdin
                .map(|value| self.runtime_handles.process.insert_stream(value))
                .transpose()?;
            let stdout = self.runtime_handles.process.insert_stream(spawned.stdout)?;
            let stderr = spawned
                .stderr
                .map(|value| self.runtime_handles.process.insert_stream(value))
                .transpose()?;
            return Ok(Spawned {
                child,
                stdin,
                stdout,
                stderr,
            });
        }
        #[cfg(windows)]
        {
            let spawned = windows::spawn(
                self.runtime_handles
                    .process
                    .groups
                    .get_mut(&group.rep())
                    .ok_or_else(|| "unknown execution group".to_owned())?,
                request,
            )?;
            let child = self
                .runtime_handles
                .process
                .insert_child(group.rep(), spawned.native_child)?;
            let stdin = spawned
                .stdin
                .map(|value| self.runtime_handles.process.insert_stream(value))
                .transpose()?;
            let stdout = self.runtime_handles.process.insert_stream(spawned.stdout)?;
            let stderr = spawned
                .stderr
                .map(|value| self.runtime_handles.process.insert_stream(value))
                .transpose()?;
            return Ok(Spawned {
                child,
                stdin,
                stdout,
                stderr,
            });
        }
        #[cfg(not(any(unix, windows)))]
        Err("process-host is unavailable on this platform".into())
    }

    fn signal(&mut self, group: Resource<ExecutionGroup>, signal: Signal) -> Result<(), String> {
        self.require_process_capability()?;
        #[cfg(unix)]
        return self
            .runtime_handles
            .process
            .groups
            .get_mut(&group.rep())
            .ok_or_else(|| "unknown execution group".to_owned())?
            .signal(signal);
        #[cfg(windows)]
        return self
            .runtime_handles
            .process
            .groups
            .get_mut(&group.rep())
            .ok_or_else(|| "unknown execution group".to_owned())?
            .signal(signal);
        #[cfg(not(any(unix, windows)))]
        Err("process-host is unavailable on this platform".into())
    }

    fn resize(
        &mut self,
        group: Resource<ExecutionGroup>,
        cols: u16,
        rows: u16,
    ) -> Result<(), String> {
        self.require_process_capability()?;
        #[cfg(unix)]
        return self
            .runtime_handles
            .process
            .groups
            .get_mut(&group.rep())
            .ok_or_else(|| "unknown execution group".to_owned())?
            .resize(cols, rows);
        #[cfg(windows)]
        return self
            .runtime_handles
            .process
            .groups
            .get_mut(&group.rep())
            .ok_or_else(|| "unknown execution group".to_owned())?
            .resize(cols, rows);
        #[cfg(not(any(unix, windows)))]
        Err("process-host is unavailable on this platform".into())
    }

    fn close(&mut self, group: Resource<ExecutionGroup>) {
        #[cfg(unix)]
        if let Some(value) = self.runtime_handles.process.groups.get_mut(&group.rep()) {
            value.close();
        }
        #[cfg(windows)]
        if let Some(value) = self.runtime_handles.process.groups.get_mut(&group.rep()) {
            value.close();
        }
    }

    fn drop(&mut self, group: Resource<ExecutionGroup>) -> wasmtime::Result<()> {
        #[cfg(unix)]
        if let Some(mut value) = self.runtime_handles.process.groups.remove(&group.rep()) {
            value.close();
            return Ok(());
        }
        #[cfg(windows)]
        if let Some(mut value) = self.runtime_handles.process.groups.remove(&group.rep()) {
            value.close();
            return Ok(());
        }
        Err(wasmtime::Error::msg("unknown execution group"))
    }
}

impl HostChild for HostState {
    fn state(&mut self, child: Resource<Child>) -> State {
        self.child_poll(&child)
            .ok()
            .flatten()
            .map_or(State::Running, |_| State::Exited)
    }

    fn poll_exit(&mut self, child: Resource<Child>) -> Result<Option<NativeExit>, String> {
        self.child_poll(&child)
    }

    fn drop(&mut self, child: Resource<Child>) -> wasmtime::Result<()> {
        self.runtime_handles
            .process
            .children
            .remove(&child.rep())
            .ok_or_else(|| wasmtime::Error::msg("unknown child handle"))?;
        Ok(())
    }
}

impl HostState {
    fn require_process_capability(&self) -> Result<(), String> {
        matches!(
            self.plugin_id(),
            "ohrats:shell" | "ohrats:execution-runtime"
        )
        .then_some(())
        .ok_or_else(|| "component is not granted process-host".into())
    }

    fn child_poll(&mut self, child: &Resource<Child>) -> Result<Option<NativeExit>, String> {
        let child_ref = self
            .runtime_handles
            .process
            .children
            .get(&child.rep())
            .ok_or_else(|| "unknown child handle".to_owned())?;
        #[cfg(any(unix, windows))]
        {
            self.runtime_handles
                .process
                .groups
                .get_mut(&child_ref.group)
                .ok_or_else(|| "execution group unavailable".to_owned())?
                .poll(child_ref.native)
        }
        #[cfg(not(any(unix, windows)))]
        Err("process-host is unavailable on this platform".into())
    }
}
