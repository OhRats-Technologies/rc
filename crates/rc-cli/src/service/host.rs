#![windows_subsystem = "windows"]
mod host_job;
mod host_retry;
#[path = "../../../rc-platform/src/service_log.rs"]
#[allow(dead_code)]
mod service_log;

use std::{
    io::{self, Read},
    os::windows::process::CommandExt,
    path::PathBuf,
    process::{Command, Stdio},
    sync::{Arc, Mutex},
    thread,
    time::Duration,
};
type Log = Arc<Mutex<service_log::Journal>>;

fn main() {
    let code = run().unwrap_or(1);
    std::process::exit(code);
}

fn log(journal: &Log, message: &str) -> io::Result<()> {
    journal
        .lock()
        .map_err(|_| io::Error::other("service log lock failed"))?
        .append(message)
}

fn pump(
    mut input: impl Read + Send + 'static,
    journal: Log,
    stream: &'static str,
) -> thread::JoinHandle<io::Result<()>> {
    thread::spawn(move || {
        let mut buffer = [0u8; 1024];
        let mut line = Vec::new();
        loop {
            let count = input.read(&mut buffer)?;
            if count == 0 {
                break;
            }
            for byte in &buffer[..count] {
                if *byte == b'\n' || line.len() == service_log::MAX_MESSAGE {
                    log(
                        &journal,
                        &format!("{stream}: {}", String::from_utf8_lossy(&line)),
                    )?;
                    line.clear();
                }
                if *byte != b'\n' {
                    line.push(*byte);
                }
            }
        }
        if !line.is_empty() {
            log(
                &journal,
                &format!("{stream}: {}", String::from_utf8_lossy(&line)),
            )?;
        }
        Ok(())
    })
}

fn run() -> io::Result<i32> {
    let mut args = std::env::args_os().skip(1);
    let mode = args
        .next()
        .ok_or_else(|| io::Error::other("mode missing"))?;
    let directory = PathBuf::from(
        args.next()
            .ok_or_else(|| io::Error::other("state directory missing"))?,
    );
    let journal = Arc::new(Mutex::new(service_log::Journal::new(&directory)?));
    let result = if mode == "--worker" {
        supervise(&directory, args.collect(), &journal)
    } else if mode == "--supervise" {
        host_retry::run(&directory, args.collect(), &journal)
    } else {
        Err(io::Error::other("invalid service host mode"))
    };
    if let Err(error) = &result {
        let _ = log(&journal, &format!("service host failed: {error}"));
    }
    result
}

fn supervise(
    directory: &std::path::Path,
    args: Vec<std::ffi::OsString>,
    journal: &Log,
) -> io::Result<i32> {
    let (kernel, arguments) = args
        .split_first()
        .ok_or_else(|| io::Error::other("kernel missing"))?;
    host_job::contain_process_tree()?;
    let stop = directory.join("service.stop");
    if stop.exists() {
        return Ok(0);
    }
    log(journal, "service starting (windowless)")?;
    let mut child = Command::new(kernel)
        .args(arguments)
        .creation_flags(0x08000000) // CREATE_NO_WINDOW
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .env("NO_COLOR", "1")
        .spawn()?;
    log(journal, &format!("Node started, pid={}", child.id()))?;
    let stdout = pump(child.stdout.take().unwrap(), journal.clone(), "stdout");
    let stderr = pump(child.stderr.take().unwrap(), journal.clone(), "stderr");
    let mut stopped = false;
    let status = loop {
        if let Some(status) = child.try_wait()? {
            break status;
        }
        if stop.exists() {
            log(journal, "service stop requested")?;
            stopped = true;
            child.kill()?;
            break child.wait()?;
        }
        thread::sleep(Duration::from_millis(100));
    };
    // Descendants may retain inherited pipe handles after the Node exits.
    // Bound draining; ExitProcess then closes our job and kills the whole tree.
    let deadline = std::time::Instant::now() + Duration::from_millis(500);
    while (!stdout.is_finished() || !stderr.is_finished()) && std::time::Instant::now() < deadline {
        thread::sleep(Duration::from_millis(10));
    }
    for reader in [stdout, stderr] {
        if reader.is_finished() {
            reader
                .join()
                .map_err(|_| io::Error::other("service output reader failed"))??;
        }
    }
    log(journal, &format!("Node exited: {status}"))?;
    if stopped {
        log(journal, "service stopped")?;
        return Ok(0);
    }
    // Preserve a clean exit (including revoked enrollment); retry failures only.
    Ok(status.code().unwrap_or(1))
}
