use super::storage_fs;
use crate::host::HostEnvironment;
use std::{
    fs,
    io::{BufRead, BufReader},
    path::Path,
};

const TRANSACTIONS: &str = "component-transactions";

pub(super) fn recover(environment: &HostEnvironment) -> anyhow::Result<()> {
    let root = environment.cache_dir.join(TRANSACTIONS);
    let Ok(entries) = fs::read_dir(&root) else {
        return Ok(());
    };
    for entry in entries {
        let path = entry?.path();
        if !path.is_dir() {
            continue;
        }
        match read_phase(&path).ok().as_deref() {
            Some("committing") => {
                rollback(environment, &path, &read_names(&path.join("changed"))?)?;
                fs::remove_dir_all(path)?;
            }
            Some("committed") => fs::remove_dir_all(path)?,
            // Prepared transactions remain valid until their caller commits or
            // aborts them. Recovery must not invalidate another prepared set.
            Some("prepared") => {}
            _ => fs::remove_dir_all(path)?,
        }
    }
    Ok(())
}

pub(super) fn rollback(
    environment: &HostEnvironment,
    directory: &Path,
    changed: &[String],
) -> anyhow::Result<()> {
    let backup = directory.join("backup");
    let id = directory
        .file_name()
        .and_then(|value| value.to_str())
        .unwrap_or("");
    for name in changed.iter().rev() {
        let component = storage_fs::component_path(environment, name);
        let marker = storage_fs::marker_path(environment, name);
        let old_component = backup.join(format!("{name}.wasm"));
        let old_marker = backup.join(format!("{name}.managed"));
        if old_component.is_file() {
            storage_fs::atomic_write(&component, &fs::read(old_component)?)?;
        } else {
            remove_if_present(&component)?;
        }
        if old_marker.is_file() {
            storage_fs::atomic_write(&marker, &fs::read(old_marker)?)?;
        } else {
            remove_if_present(&marker)?;
        }
        remove_if_present(&temp_path(environment, id, name, "wasm"))?;
        remove_if_present(&temp_path(environment, id, name, "managed"))?;
    }
    Ok(())
}

pub(super) fn read_names(path: &Path) -> anyhow::Result<Vec<String>> {
    let file = fs::File::open(path)?;
    Ok(BufReader::new(file)
        .lines()
        .map(|line| line.map_err(Into::into))
        .filter(|line| line.as_ref().is_ok_and(|value| !value.is_empty()))
        .collect::<Result<_, anyhow::Error>>()?)
}

pub(super) fn read_phase(path: &Path) -> anyhow::Result<String> {
    Ok(fs::read_to_string(path)?.trim().to_owned())
}

fn temp_path(
    environment: &HostEnvironment,
    id: &str,
    name: &str,
    suffix: &str,
) -> std::path::PathBuf {
    environment
        .component_dir
        .join(format!(".rc-update-{id}-{name}.{suffix}.tmp"))
}

fn remove_if_present(path: &Path) -> anyhow::Result<()> {
    match fs::remove_file(path) {
        Ok(()) => Ok(()),
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(()),
        Err(error) => Err(error.into()),
    }
}
