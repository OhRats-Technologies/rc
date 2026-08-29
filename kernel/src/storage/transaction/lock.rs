use crate::host::HostEnvironment;
use std::{
    fs::{self, File, OpenOptions},
    io::{ErrorKind, Write as _},
    path::PathBuf,
};

pub(crate) const FENCE: &str = ".rc-component-update";
const LOCK: &str = "component-transactions.lock";

pub(super) struct Recovery {
    _file: File,
}

impl Recovery {
    pub(super) fn acquire(environment: &HostEnvironment) -> anyhow::Result<Self> {
        let file = acquire(environment)?;
        remove_if_present(&environment.component_dir.join(FENCE))?;
        Ok(Self { _file: file })
    }
}

pub(super) struct Publication {
    _file: File,
    fence: PathBuf,
}

impl Publication {
    pub(super) fn acquire(environment: &HostEnvironment) -> anyhow::Result<Self> {
        let file = acquire(environment)?;
        let fence = environment.component_dir.join(FENCE);
        remove_if_present(&fence)?;
        let mut marker = OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(&fence)?;
        writeln!(marker, "{}", std::process::id())?;
        marker.sync_all()?;
        Ok(Self { _file: file, fence })
    }
}

impl Drop for Publication {
    fn drop(&mut self) {
        let _ = fs::remove_file(&self.fence);
    }
}

fn acquire(environment: &HostEnvironment) -> anyhow::Result<File> {
    fs::create_dir_all(environment.cache_dir.as_ref())?;
    let file = OpenOptions::new()
        .read(true)
        .write(true)
        .create(true)
        .truncate(false)
        .open(environment.cache_dir.join(LOCK))?;
    file.try_lock()
        .map_err(|error| anyhow::anyhow!("cannot acquire component transaction lock: {error}"))?;
    Ok(file)
}

fn remove_if_present(path: &std::path::Path) -> anyhow::Result<()> {
    match fs::remove_file(path) {
        Ok(()) => Ok(()),
        Err(error) if error.kind() == ErrorKind::NotFound => Ok(()),
        Err(error) => Err(error.into()),
    }
}
