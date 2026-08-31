use crate::{
    bindings::ohrats::rc_process::filesystem_host::{Entry, EntryKind, Host},
    host::HostState,
};
use std::{
    fs::{self, OpenOptions},
    io::{Read, Write},
    path::Path,
};

impl Host for HostState {
    fn read(&mut self, path: String, max_bytes: u64) -> Result<Vec<u8>, String> {
        self.require_runtime_capability("filesystem")?;
        let maximum = usize::try_from(max_bytes)
            .map_err(|_| "filesystem read capacity exceeds platform limits".to_owned())?;
        let mut file = fs::File::open(valid_path(&path)?).map_err(display)?;
        let mut bytes = Vec::new();
        Read::by_ref(&mut file)
            .take(max_bytes.saturating_add(1))
            .read_to_end(&mut bytes)
            .map_err(display)?;
        if bytes.len() > maximum {
            return Err("file exceeds requested read capacity".into());
        }
        Ok(bytes)
    }

    fn write(&mut self, path: String, bytes: Vec<u8>, append: bool) -> Result<(), String> {
        self.require_runtime_capability("filesystem")?;
        let mut options = OpenOptions::new();
        options.create(true).write(true);
        if append {
            options.append(true);
        } else {
            options.truncate(true);
        }
        options
            .open(valid_path(&path)?)
            .and_then(|mut file| file.write_all(&bytes))
            .map_err(display)
    }

    fn list_directory(&mut self, path: String, max_entries: u32) -> Result<Vec<Entry>, String> {
        self.require_runtime_capability("filesystem")?;
        let maximum = usize::try_from(max_entries)
            .map_err(|_| "directory entry capacity exceeds platform limits".to_owned())?;
        let mut entries = Vec::new();
        for value in fs::read_dir(valid_path(&path)?).map_err(display)? {
            if entries.len() == maximum {
                return Err("directory exceeds requested entry capacity".into());
            }
            let value = value.map_err(display)?;
            let metadata = fs::symlink_metadata(value.path()).map_err(display)?;
            entries.push(Entry {
                name: value
                    .file_name()
                    .into_string()
                    .map_err(|_| "directory entry name is not Unicode".to_owned())?,
                kind: entry_kind(&metadata),
            });
        }
        Ok(entries)
    }

    fn create_directory(&mut self, path: String, recursive: bool) -> Result<(), String> {
        self.require_runtime_capability("filesystem")?;
        if recursive {
            fs::create_dir_all(valid_path(&path)?).map_err(display)
        } else {
            fs::create_dir(valid_path(&path)?).map_err(display)
        }
    }

    fn remove(&mut self, path: String, recursive: bool) -> Result<(), String> {
        self.require_runtime_capability("filesystem")?;
        remove_path(valid_path(&path)?, recursive)
    }

    fn rename(&mut self, source: String, destination: String) -> Result<(), String> {
        self.require_runtime_capability("filesystem")?;
        fs::rename(valid_path(&source)?, valid_path(&destination)?).map_err(display)
    }

    fn current_directory(&mut self) -> Result<String, String> {
        self.require_runtime_capability("filesystem")?;
        std::env::current_dir()
            .map_err(display)?
            .into_os_string()
            .into_string()
            .map_err(|_| "current directory is not Unicode".into())
    }
}

fn remove_path(path: &Path, recursive: bool) -> Result<(), String> {
    let metadata = fs::symlink_metadata(path).map_err(display)?;
    if link_like(&metadata) {
        if directory_like(&metadata) {
            fs::remove_dir(path).map_err(display)
        } else {
            fs::remove_file(path).map_err(display)
        }
    } else if metadata.is_dir() {
        if recursive {
            fs::remove_dir_all(path).map_err(display)
        } else {
            fs::remove_dir(path).map_err(display)
        }
    } else {
        fs::remove_file(path).map_err(display)
    }
}

#[cfg(windows)]
fn directory_like(metadata: &fs::Metadata) -> bool {
    use std::os::windows::fs::MetadataExt as _;
    const FILE_ATTRIBUTE_DIRECTORY: u32 = 0x10;
    metadata.file_attributes() & FILE_ATTRIBUTE_DIRECTORY != 0
}

#[cfg(not(windows))]
fn directory_like(metadata: &fs::Metadata) -> bool {
    metadata.is_dir()
}

fn valid_path(value: &str) -> Result<&Path, String> {
    if value.is_empty() || value.contains('\0') {
        return Err("invalid filesystem path".into());
    }
    Ok(Path::new(value))
}

fn entry_kind(metadata: &fs::Metadata) -> EntryKind {
    let kind = metadata.file_type();
    if link_like(metadata) {
        EntryKind::Symlink
    } else if kind.is_file() {
        EntryKind::File
    } else if kind.is_dir() {
        EntryKind::Directory
    } else {
        EntryKind::Other
    }
}

#[cfg(windows)]
fn link_like(metadata: &fs::Metadata) -> bool {
    use std::os::windows::fs::MetadataExt as _;
    const FILE_ATTRIBUTE_REPARSE_POINT: u32 = 0x400;
    metadata.file_type().is_symlink()
        || metadata.file_attributes() & FILE_ATTRIBUTE_REPARSE_POINT != 0
}

#[cfg(not(windows))]
fn link_like(metadata: &fs::Metadata) -> bool {
    metadata.file_type().is_symlink()
}

fn display(error: std::io::Error) -> String {
    error.to_string()
}

#[cfg(test)]
mod tests;
