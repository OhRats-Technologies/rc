use super::events::emit_to;
use super::{
    EventSink, ManagedProcess, ProcessInput, ProcessSpec, RelaySink, SecureSink, SecureState,
};
use crate::process::{ProcessEvent, StreamKind};
use nix::{
    fcntl::{FcntlArg, FdFlag, fcntl},
    pty::{Winsize, openpty},
    unistd::{pipe, setsid},
};
use parking_lot::Mutex;
use std::{
    fs::File,
    io::{self, Read},
    os::{fd::AsRawFd, unix::process::CommandExt},
    path::PathBuf,
    process::{Command, Stdio},
    sync::Arc,
};

type Reader = Box<dyn Read + Send>;
type Spawned = (
    ManagedProcess,
    std::process::Child,
    Vec<(StreamKind, Reader)>,
);

pub(super) fn spawn(runner: &PathBuf, spec: &ProcessSpec) -> io::Result<Spawned> {
    let (lifeline_read, lifeline_write) = pipe().map_err(io::Error::other)?;
    fcntl(&lifeline_read, FcntlArg::F_SETFD(FdFlag::FD_CLOEXEC)).map_err(io::Error::other)?;
    fcntl(&lifeline_write, FcntlArg::F_SETFD(FdFlag::FD_CLOEXEC)).map_err(io::Error::other)?;
    let lifeline_fd = lifeline_read.as_raw_fd();
    let mut command = Command::new(runner);
    command
        .arg("__process-runner")
        .env("OHRATS_PROCESS_COMMAND", &spec.command)
        .env("OHRATS_PROCESS_CWD", &spec.cwd)
        .env("OHRATS_LIFELINE_FD", lifeline_fd.to_string());
    let mut pty_master = None;
    if let Some(term) = &spec.terminal {
        let (cols, rows) = bounded_size(term.cols, term.rows);
        let pty = openpty(
            Some(&Winsize {
                ws_row: rows,
                ws_col: cols,
                ws_xpixel: 0,
                ws_ypixel: 0,
            }),
            None,
        )
        .map_err(io::Error::other)?;
        let stdin_fd = nix::unistd::dup(&pty.slave).map_err(io::Error::other)?;
        let stdout_fd = nix::unistd::dup(&pty.slave).map_err(io::Error::other)?;
        command
            .stdin(Stdio::from(File::from(stdin_fd)))
            .stdout(Stdio::from(File::from(stdout_fd)))
            .stderr(Stdio::from(File::from(pty.slave)));
        command.env("OHRATS_PROCESS_TERMINAL", "1").env(
            "TERM",
            if term.term.trim().is_empty() {
                "xterm-256color"
            } else {
                term.term.trim()
            },
        );
        pty_master = Some(File::from(pty.master));
    } else {
        command
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped());
    }
    let has_tty = pty_master.is_some();
    unsafe {
        command.pre_exec(move || {
            setsid().map_err(io::Error::other)?;
            if nix::libc::fcntl(lifeline_fd, nix::libc::F_SETFD, 0) < 0 {
                return Err(io::Error::last_os_error());
            }
            if has_tty && nix::libc::ioctl(0, nix::libc::TIOCSCTTY as _, 0) < 0 {
                return Err(io::Error::last_os_error());
            }
            Ok(())
        });
    }
    let mut child = command.spawn()?;
    drop(lifeline_read);
    let lifeline = File::from(lifeline_write);
    let pid = child.id() as i32;
    let mut readers = Vec::new();
    let input = if let Some(master) = pty_master {
        readers.push((StreamKind::Stdout, Box::new(master.try_clone()?) as Reader));
        ProcessInput::Pty(master)
    } else {
        let stdout = child
            .stdout
            .take()
            .ok_or_else(|| io::Error::other("process stdout was not piped"))?;
        let stderr = child
            .stderr
            .take()
            .ok_or_else(|| io::Error::other("process stderr was not piped"))?;
        readers.push((StreamKind::Stdout, Box::new(stdout) as Reader));
        readers.push((StreamKind::Stderr, Box::new(stderr) as Reader));
        ProcessInput::Pipe(child.stdin.take())
    };
    Ok((
        ManagedProcess {
            pid,
            input: Mutex::new(input),
            lifeline: Mutex::new(Some(lifeline)),
            secure: spec.secure,
            user_id: spec.user_id.clone(),
            relay_id: spec.relay_id.clone(),
            secure_state: Mutex::new(SecureState {
                session_id: spec.session_id.clone(),
                ..Default::default()
            }),
        },
        child,
        readers,
    ))
}

pub(super) fn capture_reader(
    mut reader: Reader,
    kind: StreamKind,
    id: String,
    process: Arc<ManagedProcess>,
    event_sink: EventSink,
    secure_sink: Arc<Mutex<Option<SecureSink>>>,
    relay_sink: Arc<Mutex<Option<RelaySink>>>,
) {
    let mut buffer = [0_u8; 16 * 1024];
    loop {
        match reader.read(&mut buffer) {
            Ok(0) | Err(_) => break,
            Ok(n) => emit_to(
                &event_sink,
                &secure_sink,
                &relay_sink,
                &process,
                ProcessEvent::output(kind, &id, &buffer[..n]),
            ),
        }
    }
}

fn bounded_size(cols: u16, rows: u16) -> (u16, u16) {
    (
        if (2..=500).contains(&cols) { cols } else { 80 },
        if (2..=500).contains(&rows) { rows } else { 24 },
    )
}

pub(super) fn set_terminal_size(file: &File, cols: u16, rows: u16) -> io::Result<()> {
    let (cols, rows) = bounded_size(cols, rows);
    let size = nix::libc::winsize {
        ws_row: rows,
        ws_col: cols,
        ws_xpixel: 0,
        ws_ypixel: 0,
    };
    let result = unsafe { nix::libc::ioctl(file.as_raw_fd(), nix::libc::TIOCSWINSZ as _, &size) };
    if result < 0 {
        Err(io::Error::last_os_error())
    } else {
        Ok(())
    }
}

pub(super) fn exit_result(status: io::Result<std::process::ExitStatus>) -> (i32, String) {
    use std::os::unix::process::ExitStatusExt;
    match status {
        Ok(status) => (
            status.code().unwrap_or(-1),
            status
                .signal()
                .map(crate::process::session::signal_name)
                .unwrap_or_default(),
        ),
        Err(_) => (-1, String::new()),
    }
}
