use std::{
    fs::{self, File, OpenOptions},
    io,
    path::{Path, PathBuf},
};

#[cfg(unix)]
use std::os::fd::AsRawFd;

pub struct RunLock {
    _file: File,
}

pub fn node_lock_path(dir: &Path) -> PathBuf {
    dir.join("node.lock")
}

pub fn acquire_run_lock(dir: &Path) -> io::Result<RunLock> {
    fs::create_dir_all(dir)?;
    let path = node_lock_path(dir);
    let mut options = OpenOptions::new();
    options.create(true).truncate(false).read(true).write(true);
    #[cfg(windows)]
    {
        use std::os::windows::fs::OpenOptionsExt as _;
        options.share_mode(0);
    }
    let file = options.open(path).map_err(lock_error)?;
    #[cfg(unix)]
    {
        let result =
            unsafe { nix::libc::flock(file.as_raw_fd(), nix::libc::LOCK_EX | nix::libc::LOCK_NB) };
        if result != 0 {
            let error = io::Error::last_os_error();
            if matches!(
                error.raw_os_error(),
                Some(code) if code == nix::libc::EWOULDBLOCK || code == nix::libc::EAGAIN
            ) {
                return Err(already_running());
            }
            return Err(error);
        }
    }
    Ok(RunLock { _file: file })
}

fn lock_error(error: io::Error) -> io::Error {
    #[cfg(windows)]
    if matches!(error.raw_os_error(), Some(32 | 33)) {
        return already_running();
    }
    error
}

fn already_running() -> io::Error {
    io::Error::new(
        io::ErrorKind::AlreadyExists,
        "RC Node is already running for this enrollment; stop the service or exit the other `rc run` first",
    )
}
