use sha2::{Digest, Sha256};
use std::{
    fs::{self, File},
    io::{Read, Write},
    path::{Path, PathBuf},
    process::{Command, Stdio},
    thread,
    time::{Duration, Instant},
};

const JOURNAL: &str = ".rc-kernel-replacement.journal";
const HEALTH_LIMIT: u64 = 4097;
const HEALTH_TIMEOUT: Duration = Duration::from_secs(5);

pub(super) fn commit(stage: &Path, target: &Path, expected: &str) -> Result<(), String> {
    let parent = target
        .parent()
        .ok_or_else(|| "kernel has no parent".to_owned())?;
    let backup = parent.join(format!(".rc-kernel-backup-{}", std::process::id()));
    let journal = parent.join(JOURNAL);
    fs::copy(target, &backup).map_err(|error| format!("backup kernel: {error}"))?;
    sync_file(&backup)?;
    write_journal(&journal, stage, &backup, expected)?;
    fs::rename(stage, target).map_err(|error| format!("atomically replace kernel: {error}"))?;
    sync_file(target)?;
    sync_parent(parent)
}

pub(super) fn recover(parent: &Path, target: &Path) -> Result<(), String> {
    let journal = parent.join(JOURNAL);
    if !journal.exists() {
        return Ok(());
    }
    let fields = fs::read_to_string(&journal)
        .map_err(|error| format!("read replacement journal: {error}"))?;
    let mut lines = fields.lines();
    let stage = PathBuf::from(
        lines
            .next()
            .ok_or_else(|| "invalid replacement journal".to_owned())?,
    );
    let backup = PathBuf::from(
        lines
            .next()
            .ok_or_else(|| "invalid replacement journal".to_owned())?,
    );
    let expected = lines
        .next()
        .ok_or_else(|| "invalid replacement journal".to_owned())?;
    if target.exists()
        && digest_file(target).ok().as_deref() == Some(expected)
        && verify_kernel(target).is_ok()
    {
        let _ = fs::remove_file(stage);
        let _ = fs::remove_file(backup);
        let _ = fs::remove_file(journal);
        return sync_parent(parent);
    }
    if backup.exists() {
        fs::rename(backup, target).map_err(|error| format!("restore kernel backup: {error}"))?;
    }
    let _ = fs::remove_file(stage);
    let _ = fs::remove_file(journal);
    sync_parent(parent)
}

pub(super) fn write_executable(path: &Path, bytes: &[u8]) -> Result<(), String> {
    let mut file = File::create(path).map_err(|error| format!("stage kernel: {error}"))?;
    file.write_all(bytes)
        .map_err(|error| format!("write staged kernel: {error}"))?;
    file.sync_all()
        .map_err(|error| format!("sync staged kernel: {error}"))?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(path, fs::Permissions::from_mode(0o755))
            .map_err(|error| format!("set staged kernel permissions: {error}"))?;
    }
    Ok(())
}

fn write_journal(journal: &Path, stage: &Path, backup: &Path, digest: &str) -> Result<(), String> {
    let temporary = journal.with_extension(format!("tmp-{}", std::process::id()));
    write_executable(
        &temporary,
        format!("{}\n{}\n{}\n", stage.display(), backup.display(), digest).as_bytes(),
    )?;
    fs::rename(temporary, journal).map_err(|error| format!("publish replacement journal: {error}"))
}

pub(super) fn verify_kernel(path: &Path) -> Result<String, String> {
    let mut child = Command::new(path)
        .arg("--version")
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .map_err(|error| format!("run staged kernel health check: {error}"))?;
    let stdout = child.stdout.take().ok_or("capture kernel version")?;
    let stderr = child.stderr.take().ok_or("capture kernel diagnostics")?;
    let stdout = thread::spawn(move || read_bounded(stdout));
    let stderr = thread::spawn(move || read_bounded(stderr));
    let deadline = Instant::now() + HEALTH_TIMEOUT;
    let status = loop {
        if let Some(status) = child
            .try_wait()
            .map_err(|error| format!("wait for kernel health check: {error}"))?
        {
            break status;
        }
        if Instant::now() >= deadline {
            let _ = child.kill();
            let _ = child.wait();
            return Err("staged kernel health check timed out".into());
        }
        thread::sleep(Duration::from_millis(10));
    };
    let stdout = stdout
        .join()
        .map_err(|_| "capture kernel version failed".to_owned())??;
    let stderr = stderr
        .join()
        .map_err(|_| "capture kernel diagnostics failed".to_owned())??;
    if !status.success()
        || stdout.len() >= HEALTH_LIMIT as usize
        || stderr.len() >= HEALTH_LIMIT as usize
    {
        return Err("staged kernel failed its bounded health check".into());
    }
    let text = String::from_utf8(stdout).map_err(|_| "kernel version is not UTF-8".to_owned())?;
    let version = text
        .trim()
        .strip_prefix("RC kernel ")
        .ok_or_else(|| "kernel version prefix is invalid".to_owned())?;
    semver::Version::parse(version).map_err(|_| "kernel version is not semantic".to_owned())?;
    Ok(version.into())
}

pub(super) fn validate_digest(value: &str) -> Result<(), String> {
    if value.len() != 71
        || !value.starts_with("sha256:")
        || !value[7..]
            .bytes()
            .all(|byte| byte.is_ascii_digit() || matches!(byte, b'a'..=b'f'))
    {
        return Err("expected sha256:<64 hexadecimal digits>".into());
    }
    Ok(())
}

fn read_bounded(stream: impl Read) -> Result<Vec<u8>, String> {
    let mut bytes = Vec::new();
    stream
        .take(HEALTH_LIMIT)
        .read_to_end(&mut bytes)
        .map_err(|error| format!("read kernel health output: {error}"))?;
    Ok(bytes)
}

pub(super) fn digest(bytes: &[u8]) -> String {
    format!("sha256:{:x}", Sha256::digest(bytes))
}

pub(super) fn digest_file(path: &Path) -> Result<String, String> {
    fs::read(path)
        .map(|bytes| digest(&bytes))
        .map_err(|error| format!("read kernel: {error}"))
}

fn sync_file(path: &Path) -> Result<(), String> {
    File::open(path)
        .map_err(|error| format!("open replacement: {error}"))?
        .sync_all()
        .map_err(|error| format!("sync replacement: {error}"))
}

fn sync_parent(path: &Path) -> Result<(), String> {
    #[cfg(unix)]
    File::open(path)
        .map_err(|error| format!("open replacement directory: {error}"))?
        .sync_all()
        .map_err(|error| format!("sync replacement directory: {error}"))?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::{digest, recover, verify_kernel};
    use std::{fs, os::unix::fs::PermissionsExt, path::Path};

    #[test]
    fn rejects_invalid_executable() -> anyhow::Result<()> {
        let directory = tempfile::tempdir()?;
        let path = directory.path().join("candidate");
        fs::write(&path, b"not a kernel")?;
        assert!(verify_kernel(&path).is_err());
        Ok(())
    }

    #[test]
    fn accepts_candidate_reported_version() -> anyhow::Result<()> {
        let directory = tempfile::tempdir()?;
        let path = directory.path().join("candidate");
        fs::write(&path, b"#!/bin/sh\nprintf 'RC kernel 0.1.1\\n'\n")?;
        fs::set_permissions(&path, fs::Permissions::from_mode(0o755))?;
        assert_eq!(verify_kernel(&path).map_err(anyhow::Error::msg)?, "0.1.1");
        Ok(())
    }

    #[test]
    fn interrupted_post_commit_restores_backup() -> anyhow::Result<()> {
        let directory = tempfile::tempdir()?;
        let target = directory.path().join("rc-kernel");
        let backup = directory.path().join("backup");
        let stage = directory.path().join("stage");
        fs::write(&target, b"broken")?;
        fs::write(&backup, b"healthy")?;
        fs::write(&stage, b"candidate")?;
        fs::write(
            directory.path().join(".rc-kernel-replacement.journal"),
            format!(
                "{}\n{}\n{}\n",
                stage.display(),
                backup.display(),
                digest(b"candidate")
            ),
        )?;
        recover(directory.path(), Path::new(&target)).map_err(anyhow::Error::msg)?;
        assert_eq!(fs::read(target)?, b"healthy");
        Ok(())
    }
}
