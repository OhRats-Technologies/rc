use crate::process::session::stop_session;
use nix::sys::signal::Signal;
use signal_hook::{consts::SIGTERM, iterator::Signals};
use std::{
    fs::File,
    io::Read,
    os::fd::FromRawFd,
    os::unix::process::ExitStatusExt,
    path::PathBuf,
    process::{Command, Stdio},
    sync::mpsc,
};

pub fn run_process_runner() -> i32 {
    let command = std::env::var("OHRATS_PROCESS_COMMAND").unwrap_or_default();
    if command.is_empty() {
        return 127;
    }
    let lifeline_fd = std::env::var("OHRATS_LIFELINE_FD")
        .ok()
        .and_then(|value| value.parse::<i32>().ok());
    let Some(lifeline_fd) = lifeline_fd else {
        return 127;
    };
    let mut lifeline = unsafe { File::from_raw_fd(lifeline_fd) };
    let session_id = std::process::id() as i32;
    let mut child = Command::new("sh");
    child
        .args(["-lc", &command])
        .stdin(Stdio::inherit())
        .stdout(Stdio::inherit())
        .stderr(Stdio::inherit());
    let cwd = process_cwd(std::env::var("OHRATS_PROCESS_CWD").unwrap_or_default());
    if !cwd.as_os_str().is_empty() {
        child.current_dir(cwd);
    }
    if std::env::var_os("OHRATS_PROCESS_TERMINAL").is_some() {
        child.env("COLORTERM", "truecolor");
    }
    let Ok(mut child) = child.spawn() else {
        return 127;
    };
    let (tx, rx) = mpsc::channel::<RunnerEvent>();
    let child_tx = tx.clone();
    std::thread::spawn(move || {
        let _ = child_tx.send(RunnerEvent::Child(child.wait()));
    });
    let line_tx = tx.clone();
    std::thread::spawn(move || {
        let mut byte = [0_u8; 1];
        if lifeline.read(&mut byte).is_err() || byte == [0] {
            let _ = line_tx.send(RunnerEvent::Lifeline);
        }
    });
    if let Ok(mut signals) = Signals::new([SIGTERM]) {
        let signal_tx = tx;
        std::thread::spawn(move || {
            if signals.forever().next().is_some() {
                let _ = signal_tx.send(RunnerEvent::Terminate);
            }
        });
    }
    match rx.recv() {
        Ok(RunnerEvent::Child(status)) => {
            stop_session(session_id, Signal::SIGTERM);
            status.map(exit_code).unwrap_or(1)
        }
        Ok(RunnerEvent::Terminate) => {
            stop_session(session_id, Signal::SIGTERM);
            143
        }
        Ok(RunnerEvent::Lifeline) | Err(_) => {
            stop_session(session_id, Signal::SIGKILL);
            137
        }
    }
}

enum RunnerEvent {
    Child(std::io::Result<std::process::ExitStatus>),
    Terminate,
    Lifeline,
}

fn exit_code(status: std::process::ExitStatus) -> i32 {
    status
        .code()
        .unwrap_or_else(|| status.signal().map(|signal| 128 + signal).unwrap_or(1))
}

fn process_cwd(value: String) -> PathBuf {
    let home = std::env::var_os("HOME")
        .map(PathBuf::from)
        .unwrap_or_default();
    if value.is_empty() || value == "~" {
        return home;
    }
    if let Some(rest) = value.strip_prefix("~/")
        && !home.as_os_str().is_empty()
    {
        return home.join(rest);
    }
    PathBuf::from(value)
}
