use super::StreamValue;
use crate::bindings::ohrats::rc_process::{
    process_host::{NativeExit, SpawnRequest},
    types::{EnvironmentBase, Signal, Terminal},
};
use nix::{
    pty::{Winsize, openpty},
    sys::signal::{Signal as UnixSignal, kill},
    unistd::Pid,
};
use std::{
    collections::BTreeMap,
    fs::File,
    io,
    os::{
        fd::{AsRawFd, OwnedFd},
        unix::process::{CommandExt, ExitStatusExt},
    },
    process::{Child, Command, Stdio},
};

#[cfg(test)]
mod tests;

#[derive(Default)]
pub struct Group {
    process_group: Option<i32>,
    terminal: Option<File>,
    children: BTreeMap<u32, Child>,
}

pub struct Spawned {
    pub native_child: u32,
    pub stdin: Option<StreamValue>,
    pub stdout: StreamValue,
    pub stderr: Option<StreamValue>,
}

pub fn spawn(group: &mut Group, request: SpawnRequest) -> Result<Spawned, String> {
    validate_request(&request)?;
    let mut command = Command::new(&request.program);
    command.args(&request.args);
    if let Some(cwd) = request.cwd.as_deref() {
        command.current_dir(cwd);
    }
    apply_environment(&mut command, request.environment);
    if let Some(terminal) = request.terminal {
        spawn_terminal(group, command, terminal)
    } else {
        spawn_piped(group, command)
    }
}

fn spawn_piped(group: &mut Group, mut command: Command) -> Result<Spawned, String> {
    command
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    let process_group = group.process_group;
    unsafe {
        command.pre_exec(move || {
            let target = process_group.unwrap_or(0);
            if libc::setpgid(0, target) < 0 {
                return Err(io::Error::last_os_error());
            }
            Ok(())
        });
    }
    let mut child = command.spawn().map_err(display)?;
    let pid: i32 = child
        .id()
        .try_into()
        .map_err(|_| "child PID exceeds i32".to_owned())?;
    group.process_group.get_or_insert(pid);
    let stdin = child
        .stdin
        .take()
        .ok_or_else(|| "child stdin was not piped".to_owned())?;
    let stdout = child
        .stdout
        .take()
        .ok_or_else(|| "child stdout was not piped".to_owned())?;
    let stderr = child
        .stderr
        .take()
        .ok_or_else(|| "child stderr was not piped".to_owned())?;
    set_nonblocking(stdin.as_raw_fd())?;
    set_nonblocking(stdout.as_raw_fd())?;
    set_nonblocking(stderr.as_raw_fd())?;
    let native_child = child.id();
    group.children.insert(native_child, child);
    Ok(Spawned {
        native_child,
        stdin: Some(StreamValue::Writer(Box::new(stdin))),
        stdout: StreamValue::Reader(Box::new(stdout)),
        stderr: Some(StreamValue::Reader(Box::new(stderr))),
    })
}

fn spawn_terminal(
    group: &mut Group,
    mut command: Command,
    terminal: Terminal,
) -> Result<Spawned, String> {
    if !group.children.is_empty() {
        return Err("terminal execution group already has a child".into());
    }
    let (cols, rows) = bounded_size(terminal.cols, terminal.rows);
    let pty = openpty(
        Some(&Winsize {
            ws_row: rows,
            ws_col: cols,
            ws_xpixel: 0,
            ws_ypixel: 0,
        }),
        None,
    )
    .map_err(display)?;
    let input = duplicate(&pty.slave)?;
    let output = duplicate(&pty.slave)?;
    command
        .stdin(Stdio::from(input))
        .stdout(Stdio::from(output))
        .stderr(Stdio::from(pty.slave))
        .env(
            "TERM",
            if terminal.term.trim().is_empty() {
                "xterm-256color"
            } else {
                terminal.term.trim()
            },
        )
        .env("COLORTERM", "truecolor");
    unsafe {
        command.pre_exec(|| {
            if libc::setsid() < 0 {
                return Err(io::Error::last_os_error());
            }
            if libc::ioctl(0, libc::TIOCSCTTY as _, 0) < 0 {
                return Err(io::Error::last_os_error());
            }
            Ok(())
        });
    }
    let child = command.spawn().map_err(display)?;
    let pid: i32 = child
        .id()
        .try_into()
        .map_err(|_| "child PID exceeds i32".to_owned())?;
    let master = File::from(pty.master);
    set_nonblocking(master.as_raw_fd())?;
    let reader = master.try_clone().map_err(display)?;
    group.process_group = Some(pid);
    group.terminal = Some(master.try_clone().map_err(display)?);
    let native_child = child.id();
    group.children.insert(native_child, child);
    Ok(Spawned {
        native_child,
        stdin: Some(StreamValue::Duplex(master)),
        stdout: StreamValue::Reader(Box::new(reader)),
        stderr: None,
    })
}

impl Group {
    pub fn poll(&mut self, child: u32) -> Result<Option<NativeExit>, String> {
        let child = self
            .children
            .get_mut(&child)
            .ok_or_else(|| "unknown native child".to_owned())?;
        child
            .try_wait()
            .map_err(display)
            .map(|value| value.map(native_exit))
    }

    pub fn signal(&mut self, signal: Signal) -> Result<(), String> {
        let Some(group) = self.process_group else {
            return Ok(());
        };
        kill(
            Pid::from_raw(-group),
            match signal {
            Signal::Interrupt => UnixSignal::SIGINT,
            Signal::Terminate => UnixSignal::SIGTERM,
            Signal::Kill => UnixSignal::SIGKILL,
            },
        )
        .map_err(display)
    }

    pub fn resize(&mut self, cols: u16, rows: u16) -> Result<(), String> {
        let terminal = self
            .terminal
            .as_ref()
            .ok_or_else(|| "execution group has no terminal".to_owned())?;
        let (cols, rows) = bounded_size(cols, rows);
        let size = libc::winsize {
            ws_row: rows,
            ws_col: cols,
            ws_xpixel: 0,
            ws_ypixel: 0,
        };
        let result = unsafe { libc::ioctl(terminal.as_raw_fd(), libc::TIOCSWINSZ as _, &size) };
        if result < 0 {
            Err(io::Error::last_os_error().to_string())
        } else {
            Ok(())
        }
    }

    pub fn close(&mut self) {
        let _ = self.signal(Signal::Kill);
        for child in self.children.values_mut() {
            let _ = child.wait();
        }
        self.children.clear();
        self.terminal = None;
    }
}

impl Drop for Group {
    fn drop(&mut self) {
        self.close();
    }
}

fn validate_request(request: &SpawnRequest) -> Result<(), String> {
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
    Ok(())
}

fn apply_environment(
    command: &mut Command,
    environment: crate::bindings::ohrats::rc_process::types::Environment,
) {
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

fn native_exit(status: std::process::ExitStatus) -> NativeExit {
    NativeExit {
        code: status.code().and_then(|value| value.try_into().ok()),
        signal: status.signal().and_then(|value| match value {
            libc::SIGINT => Some(Signal::Interrupt),
            libc::SIGTERM => Some(Signal::Terminate),
            libc::SIGKILL => Some(Signal::Kill),
            _ => None,
        }),
    }
}

fn bounded_size(cols: u16, rows: u16) -> (u16, u16) {
    (
        if (2..=500).contains(&cols) { cols } else { 80 },
        if (2..=500).contains(&rows) { rows } else { 24 },
    )
}

fn duplicate(fd: &OwnedFd) -> Result<OwnedFd, String> {
    nix::unistd::dup(fd).map_err(display)
}

fn set_nonblocking(fd: i32) -> Result<(), String> {
    let flags = unsafe { libc::fcntl(fd, libc::F_GETFL) };
    if flags < 0 || unsafe { libc::fcntl(fd, libc::F_SETFL, flags | libc::O_NONBLOCK) } < 0 {
        Err(io::Error::last_os_error().to_string())
    } else {
        Ok(())
    }
}

fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}
