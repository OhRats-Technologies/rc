use super::StreamValue;
#[path = "windows/guard.rs"]
pub(crate) mod guard;
#[path = "windows/piped.rs"]
mod piped;
#[path = "windows/support.rs"]
mod support;

use self::support::{
    SharedWriter, apply_pty_environment, assign, bounded_size, display, validate_request,
};
use crate::bindings::ohrats::rc_process::{
    process_host::{NativeExit, SpawnRequest},
    types::{Signal, Terminal},
};
use portable_pty::{CommandBuilder, MasterPty, PtySize, native_pty_system};
use std::{collections::BTreeMap, io::Write, process::Child};
use windows::{
    Win32::{
        Foundation::{CloseHandle, HANDLE},
        System::{
            Console::{CTRL_BREAK_EVENT, GenerateConsoleCtrlEvent},
            JobObjects::{
                CreateJobObjectW, JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
                JOBOBJECT_EXTENDED_LIMIT_INFORMATION, JobObjectExtendedLimitInformation,
                SetInformationJobObject, TerminateJobObject,
            },
        },
    },
    core::PCWSTR,
};

#[cfg(test)]
#[path = "windows/tests.rs"]
mod tests;

enum NativeChild {
    Piped(Child),
    Terminal(Box<dyn portable_pty::Child + Send + Sync>),
}

pub struct Group {
    job: HANDLE,
    children: BTreeMap<u32, NativeChild>,
    terminal: Option<Box<dyn MasterPty + Send>>,
    terminal_input: Option<SharedWriter>,
    process_groups: Vec<u32>,
}

// SAFETY: `Group` exclusively owns its Win32 Job Object handle. Win32 kernel
// handles may be transferred between threads, and all remaining fields are
// `Send`; access to a group remains serialized through the Wasmtime store.
unsafe impl Send for Group {}

pub struct Spawned {
    pub native_child: u32,
    pub stdin: Option<StreamValue>,
    pub stdout: StreamValue,
    pub stderr: Option<StreamValue>,
}

impl Group {
    pub fn new() -> Result<Self, String> {
        let job = unsafe { CreateJobObjectW(None, PCWSTR::null()) }.map_err(display)?;
        let mut limits = JOBOBJECT_EXTENDED_LIMIT_INFORMATION::default();
        limits.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;
        let configured = unsafe {
            SetInformationJobObject(
                job,
                JobObjectExtendedLimitInformation,
                std::ptr::from_ref(&limits).cast(),
                std::mem::size_of_val(&limits) as u32,
            )
        };
        if let Err(error) = configured {
            unsafe { CloseHandle(job) }.ok();
            return Err(display(error));
        }
        Ok(Self {
            job,
            children: BTreeMap::new(),
            terminal: None,
            terminal_input: None,
            process_groups: Vec::new(),
        })
    }

    pub fn poll(&mut self, child: u32) -> Result<Option<NativeExit>, String> {
        let child = self
            .children
            .get_mut(&child)
            .ok_or_else(|| "unknown native child".to_owned())?;
        match child {
            NativeChild::Piped(child) => child.try_wait().map_err(display).map(|status| {
                status.map(|status| NativeExit {
                    code: status.code().and_then(|code| code.try_into().ok()),
                    signal: None,
                })
            }),
            NativeChild::Terminal(child) => child.try_wait().map_err(display).map(|status| {
                status.map(|status| NativeExit {
                    code: Some(status.exit_code()),
                    signal: None,
                })
            }),
        }
    }

    pub fn signal(&mut self, signal: Signal) -> Result<(), String> {
        match signal {
            Signal::Interrupt => {
                if let Some(input) = self.terminal_input.as_mut() {
                    input.write_all(&[3]).map_err(display)
                } else if !self.process_groups.is_empty() {
                    self.console_event()
                } else {
                    Ok(())
                }
            }
            Signal::Terminate => {
                if !self.process_groups.is_empty() {
                    self.console_event()
                } else {
                    self.terminate(143)
                }
            }
            Signal::Kill => self.terminate(137),
        }
    }

    pub fn resize(&mut self, cols: u16, rows: u16) -> Result<(), String> {
        let terminal = self
            .terminal
            .as_ref()
            .ok_or_else(|| "execution group has no terminal".to_owned())?;
        let (cols, rows) = bounded_size(cols, rows);
        terminal
            .resize(PtySize {
                rows,
                cols,
                pixel_width: 0,
                pixel_height: 0,
            })
            .map_err(display)
    }

    pub fn close(&mut self) {
        let _ = self.terminate(137);
        for child in self.children.values_mut() {
            match child {
                NativeChild::Piped(child) => {
                    let _ = child.wait();
                }
                NativeChild::Terminal(child) => {
                    let _ = child.wait();
                }
            }
        }
        self.children.clear();
        self.terminal = None;
        self.terminal_input = None;
        self.process_groups.clear();
    }

    fn terminate(&self, code: u32) -> Result<(), String> {
        unsafe { TerminateJobObject(self.job, code) }.map_err(display)
    }

    fn console_event(&self) -> Result<(), String> {
        let mut first_error = None;
        for group in &self.process_groups {
            if let Err(error) = unsafe { GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, *group) } {
                first_error.get_or_insert_with(|| display(error));
            }
        }
        first_error.map_or(Ok(()), Err)
    }
}

impl Drop for Group {
    fn drop(&mut self) {
        self.close();
        unsafe { CloseHandle(self.job) }.ok();
    }
}

pub fn spawn(group: &mut Group, request: SpawnRequest) -> Result<Spawned, String> {
    validate_request(&request)?;
    if let Some(terminal) = request.terminal.clone() {
        spawn_terminal(group, request, terminal)
    } else {
        piped::spawn(group, request)
    }
}

fn spawn_terminal(
    group: &mut Group,
    request: SpawnRequest,
    terminal: Terminal,
) -> Result<Spawned, String> {
    if !group.children.is_empty() {
        return Err("terminal execution group already has a child".into());
    }
    let (cols, rows) = bounded_size(terminal.cols, terminal.rows);
    let pair = native_pty_system()
        .openpty(PtySize {
            rows,
            cols,
            pixel_width: 0,
            pixel_height: 0,
        })
        .map_err(display)?;
    let gate = guard::LaunchGate::new()?;
    let mut command = CommandBuilder::new(guard::executable()?);
    apply_pty_environment(&mut command, request.environment);
    if let Some(cwd) = request.cwd {
        command.cwd(cwd);
    }
    command.args(
        std::iter::once(guard::MARKER.to_owned())
            .chain(std::iter::once(gate.name.clone()))
            .chain(std::iter::once(gate.ready_name.clone()))
            .chain(std::iter::once(request.program))
            .chain(request.args),
    );
    let mut child = pair.slave.spawn_command(command).map_err(display)?;
    let native_child = child
        .process_id()
        .ok_or_else(|| "ConPTY child has no PID".to_owned())?;
    let handle = child
        .as_raw_handle()
        .ok_or_else(|| "ConPTY child has no handle".to_owned())?;
    if let Err(error) = assign(group.job, HANDLE(handle)) {
        let _ = child.kill();
        let _ = child.wait();
        return Err(error);
    }
    if let Err(error) = gate.wait_until_open() {
        let _ = child.kill();
        let _ = child.wait();
        return Err(error);
    }
    if let Err(error) = gate.release() {
        let _ = child.kill();
        let _ = child.wait();
        return Err(error);
    }
    let output = pair.master.try_clone_reader().map_err(display)?;
    let input = SharedWriter(std::sync::Arc::new(std::sync::Mutex::new(
        pair.master.take_writer().map_err(display)?,
    )));
    group.process_groups.push(native_child);
    group.terminal_input = Some(input.clone());
    group.terminal = Some(pair.master);
    group
        .children
        .insert(native_child, NativeChild::Terminal(child));
    Ok(Spawned {
        native_child,
        stdin: Some(StreamValue::Writer(Box::new(input))),
        stdout: StreamValue::Reader(output),
        stderr: None,
    })
}
