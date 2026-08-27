use std::{
    fs::{self, File, OpenOptions},
    io,
    os::fd::AsRawFd,
    path::{Path, PathBuf},
};

pub struct RunLock {
    _file: File,
}

pub fn node_lock_path(dir: &Path) -> PathBuf {
    dir.join("node.lock")
}

pub fn acquire_run_lock(dir: &Path) -> io::Result<RunLock> {
    fs::create_dir_all(dir)?;
    let path = node_lock_path(dir);
    let file = OpenOptions::new()
        .create(true)
        .truncate(false)
        .read(true)
        .write(true)
        .open(path)?;
    let result =
        unsafe { nix::libc::flock(file.as_raw_fd(), nix::libc::LOCK_EX | nix::libc::LOCK_NB) };
    if result != 0 {
        let error = io::Error::last_os_error();
        if matches!(
            error.raw_os_error(),
            Some(code) if code == nix::libc::EWOULDBLOCK || code == nix::libc::EAGAIN
        ) {
            return Err(io::Error::new(
                io::ErrorKind::AlreadyExists,
                "RC Node is already running for this enrollment; stop the service or exit the other `rc run` first",
            ));
        }
        return Err(error);
    }
    Ok(RunLock { _file: file })
}
