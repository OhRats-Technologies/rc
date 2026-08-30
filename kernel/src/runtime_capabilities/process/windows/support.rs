use crate::bindings::ohrats::rc_process::{
    process_host::SpawnRequest,
    types::{Environment, EnvironmentBase},
};
use portable_pty::CommandBuilder;
use std::{
    io::Write,
    process::Command,
    sync::{Arc, Mutex, MutexGuard, PoisonError},
};
use windows::Win32::{Foundation::HANDLE, System::JobObjects::AssignProcessToJobObject};

#[derive(Clone)]
pub struct SharedWriter(pub Arc<Mutex<Box<dyn Write + Send>>>);

impl Write for SharedWriter {
    fn write(&mut self, bytes: &[u8]) -> std::io::Result<usize> {
        self.0.lock().map_err(poison)?.write(bytes)
    }

    fn flush(&mut self) -> std::io::Result<()> {
        self.0.lock().map_err(poison)?.flush()
    }
}

pub fn validate_request(request: &SpawnRequest) -> Result<(), String> {
    if request.program.is_empty()
        || request.program.contains('\0')
        || request.args.iter().any(|value| value.contains('\0'))
        || request
            .cwd
            .as_deref()
            .is_some_and(|value| value.contains('\0'))
    {
        return Err("invalid native spawn request".into());
    }
    let mut names = std::collections::BTreeSet::new();
    if request.environment.changes.iter().any(|change| {
        change.name.is_empty()
            || change.name.contains(['=', '\0'])
            || change
                .value
                .as_deref()
                .is_some_and(|value| value.contains('\0'))
            || !names.insert(change.name.to_uppercase())
    }) {
        return Err("invalid or conflicting Windows environment change".into());
    }
    Ok(())
}

pub fn apply_std_environment(command: &mut Command, environment: Environment) {
    if matches!(environment.base, EnvironmentBase::Clean) {
        command.env_clear();
    }
    for change in environment.changes {
        match change.value {
            Some(value) => {
                command.env(change.name, value);
            }
            None => {
                command.env_remove(change.name);
            }
        }
    }
}

pub fn apply_pty_environment(command: &mut CommandBuilder, environment: Environment) {
    if matches!(environment.base, EnvironmentBase::Clean) {
        command.env_clear();
    }
    for change in environment.changes {
        match change.value {
            Some(value) => command.env(change.name, value),
            None => command.env_remove(change.name),
        }
    }
}

pub fn assign(job: HANDLE, process: HANDLE) -> Result<(), String> {
    unsafe { AssignProcessToJobObject(job, process) }.map_err(display)
}

pub fn bounded_size(cols: u16, rows: u16) -> (u16, u16) {
    (
        if (2..=500).contains(&cols) { cols } else { 80 },
        if (2..=500).contains(&rows) { rows } else { 24 },
    )
}

pub fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}

pub fn poison<T>(_: PoisonError<MutexGuard<'_, T>>) -> std::io::Error {
    std::io::Error::other("process stream lock is poisoned")
}
