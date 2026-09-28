//! Each attempt runs in its own kill-on-close process tree. A failed worker
//! cannot leave descendants behind when the supervisor starts the next attempt.
use super::*;

pub fn run(
    directory: &std::path::Path,
    args: Vec<std::ffi::OsString>,
    journal: &Log,
) -> io::Result<i32> {
    host_job::contain_process_tree()?;
    let stop = directory.join("service.stop");
    match std::fs::remove_file(&stop) {
        Ok(()) => {}
        Err(e) if e.kind() == io::ErrorKind::NotFound => {}
        Err(e) => return Err(e),
    }
    let mut failures = 0;
    loop {
        if stop.exists() {
            return Ok(0);
        }
        let started = std::time::Instant::now();
        let status = Command::new(std::env::current_exe()?)
            .arg("--worker")
            .arg(directory)
            .args(&args)
            .creation_flags(0x08000000)
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .status()?;
        if status.success() || stop.exists() {
            return Ok(0);
        }
        if started.elapsed() >= Duration::from_secs(300) {
            failures = 0;
        }
        failures += 1;
        if failures > 10 {
            log(journal, "service recovery exhausted after 10 retries")?;
            return Ok(1);
        }
        log(
            journal,
            &format!("service failed; retry {failures}/10 in 5 seconds"),
        )?;
        for _ in 0..50 {
            if stop.exists() {
                return Ok(0);
            }
            thread::sleep(Duration::from_millis(100));
        }
    }
}
