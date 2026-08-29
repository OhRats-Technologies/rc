#[cfg(target_os = "macos")]
use nix::unistd::getsid;
use nix::{
    sys::signal::{Signal, kill},
    unistd::Pid,
};
#[cfg(target_os = "linux")]
use std::fs;
#[cfg(target_os = "macos")]
use std::process::Command;
use std::time::Duration;

pub fn signal_session(session_id: i32, signal: Signal) {
    for pid in session_process_ids(session_id) {
        let _ = kill(Pid::from_raw(pid), signal);
    }
}

pub fn stop_session(session_id: i32, signal: Signal, terminate_grace: Duration) {
    signal_session(session_id, signal);
    if signal != Signal::SIGKILL {
        std::thread::sleep(terminate_grace);
        signal_session(session_id, Signal::SIGKILL);
    }
}

#[cfg(target_os = "linux")]
pub fn session_process_ids(session_id: i32) -> Vec<i32> {
    let self_pid = std::process::id() as i32;
    let Ok(entries) = fs::read_dir("/proc") else {
        return Vec::new();
    };
    entries
        .filter_map(Result::ok)
        .filter_map(|entry| {
            let pid = entry.file_name().to_string_lossy().parse::<i32>().ok()?;
            if pid == self_pid {
                return None;
            }
            let value = fs::read_to_string(entry.path().join("stat")).ok()?;
            let end = value.rfind(')')?;
            let fields: Vec<_> = value[end + 1..].split_whitespace().collect();
            let session = fields.get(3)?.parse::<i32>().ok()?;
            (session == session_id).then_some(pid)
        })
        .collect()
}

#[cfg(target_os = "macos")]
pub fn session_process_ids(session_id: i32) -> Vec<i32> {
    let self_pid = std::process::id() as i32;
    let Ok(output) = Command::new("/bin/ps").args(["-axo", "pid="]).output() else {
        return Vec::new();
    };
    String::from_utf8_lossy(&output.stdout)
        .split_whitespace()
        .filter_map(|field| {
            let pid = field.parse::<i32>().ok()?;
            if pid == self_pid {
                return None;
            }
            let session = getsid(Some(Pid::from_raw(pid))).ok()?.as_raw();
            (session == session_id).then_some(pid)
        })
        .collect()
}

#[cfg(not(any(target_os = "linux", target_os = "macos")))]
pub fn session_process_ids(_: i32) -> Vec<i32> {
    Vec::new()
}

pub fn signal_name(signal: i32) -> String {
    match Signal::try_from(signal) {
        Ok(Signal::SIGINT) => "SIGINT".into(),
        Ok(Signal::SIGTERM) => "SIGTERM".into(),
        Ok(Signal::SIGKILL) => "SIGKILL".into(),
        _ => format!("SIG{signal}"),
    }
}
