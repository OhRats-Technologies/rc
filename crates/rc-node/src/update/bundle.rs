use flate2::read::GzDecoder;
use sha2::{Digest, Sha256};
use std::{
    collections::BTreeMap,
    fs::{self, File},
    io::{Read, Write},
    path::{Path, PathBuf},
    process::Command,
};

const MAX_BINARY: u64 = 160 << 20;
pub(super) const MAX_COMPONENT: u64 = 48 << 20;

mod core;
use core::extract_core;

pub const CORE_COMPONENTS: &[&str] = &[
    "artifact-cache-local",
    "diagnostics-cli",
    "diagnostics-reporter",
    "diagnostics-store",
    "github-source",
    "http-source",
    "local-source",
    "oci-source",
    "package-manager",
    "process-policy",
    "transport-webrtc",
    "updater",
];

pub fn runtime_complete() -> bool {
    let Ok(executable) = std::env::current_exe() else {
        return false;
    };
    let Some(parent) = executable.parent() else {
        return false;
    };
    let Some(components) = component_dir() else {
        return false;
    };
    parent.join("rc-kernel").is_file()
        && CORE_COMPONENTS
            .iter()
            .all(|name| components.join(format!("{name}.wasm")).is_file())
}

pub fn install(
    rc_archive: Option<&[u8]>,
    kernel_archive: &[u8],
    core_archive: &[u8],
    version: &str,
) -> anyhow::Result<()> {
    let executable = std::env::current_exe()?;
    let platform_target = platform_target(&executable)?;
    let bin_dir = platform_target
        .parent()
        .ok_or_else(|| anyhow::anyhow!("invalid executable path"))?;
    let kernel_target = bin_dir.join("rc-kernel");
    let kernel = extract_single(kernel_archive, "rc-kernel", MAX_BINARY)?;
    let components = extract_core(core_archive)?;
    let component_dir = component_dir().ok_or_else(|| anyhow::anyhow!("HOME is unavailable"))?;
    fs::create_dir_all(&component_dir)?;

    let kernel_temp = bin_dir.join(format!(".rc-kernel-update-{}", std::process::id()));
    write_executable(&kernel_temp, &kernel)?;
    validate_kernel(&kernel_temp, &components, &component_dir)?;

    for (name, bytes) in components {
        install_core_component(&component_dir, &name, &bytes)?;
    }
    replace_file(&kernel_temp, &kernel_target)?;

    if let Some(archive) = rc_archive {
        let rc = extract_single(archive, "rc", MAX_BINARY)?;
        let replacement = bin_dir.join(format!(".rc-update-{}", std::process::id()));
        write_executable(&replacement, &rc)?;
        validate_rc(&replacement, version)?;
        replace_file(&replacement, &platform_target)?;
    }
    Ok(())
}

fn platform_target(executable: &Path) -> anyhow::Result<PathBuf> {
    if executable.file_name().and_then(|value| value.to_str()) == Some("rc-kernel") {
        return executable
            .parent()
            .map(|parent| parent.join("rc"))
            .ok_or_else(|| anyhow::anyhow!("invalid kernel executable path"));
    }
    Ok(executable.to_owned())
}

fn validate_kernel(
    kernel: &Path,
    components: &BTreeMap<String, Vec<u8>>,
    destination: &Path,
) -> anyhow::Result<()> {
    let output = Command::new(kernel).arg("--version").output()?;
    anyhow::ensure!(
        output.status.success()
            && String::from_utf8_lossy(&output.stdout).starts_with("RC kernel "),
        "downloaded RC kernel did not report a valid version"
    );
    let parent = destination
        .parent()
        .ok_or_else(|| anyhow::anyhow!("component directory has no parent"))?;
    let validation = parent.join(format!(".core-validation-{}", std::process::id()));
    let _ = fs::remove_dir_all(&validation);
    fs::create_dir_all(&validation)?;
    for (name, bytes) in components {
        fs::write(validation.join(format!("{name}.wasm")), bytes)?;
    }
    let status = Command::new(kernel)
        .arg("--component-dir")
        .arg(&validation)
        .arg("repair")
        .status()?;
    let _ = fs::remove_dir_all(&validation);
    anyhow::ensure!(
        status.success(),
        "RC core component bundle failed kernel validation"
    );
    Ok(())
}

fn validate_rc(path: &Path, version: &str) -> anyhow::Result<()> {
    let output = Command::new(path).arg("version").output()?;
    anyhow::ensure!(
        output.status.success()
            && String::from_utf8_lossy(&output.stdout).trim() == format!("RC {version}"),
        "downloaded executable does not match release version"
    );
    Ok(())
}

fn install_core_component(directory: &Path, name: &str, bytes: &[u8]) -> anyhow::Result<()> {
    let target = directory.join(format!("{name}.wasm"));
    let marker = directory.join(format!("{name}.core"));
    if target.exists() {
        let current = fs::read(&target)?;
        let current_digest = format!("sha256:{:x}", Sha256::digest(&current));
        let owned = fs::read_to_string(&marker).is_ok_and(|value| value.trim() == current_digest);
        if !owned {
            eprintln!(
                "preserving locally overridden component {}",
                target.display()
            );
            return Ok(());
        }
    }
    atomic_write(&target, bytes)?;
    let digest = format!("sha256:{:x}\n", Sha256::digest(bytes));
    atomic_write(&marker, digest.as_bytes())
}

fn extract_single(archive: &[u8], expected: &str, limit: u64) -> anyhow::Result<Vec<u8>> {
    let mut tar = tar::Archive::new(GzDecoder::new(archive));
    let mut value = None;
    for entry in tar.entries()? {
        let entry = entry?;
        anyhow::ensure!(
            entry.path()?.as_ref() == Path::new(expected) && value.is_none(),
            "release archive must contain only {expected}"
        );
        anyhow::ensure!(entry.header().entry_type().is_file() && entry.size() <= limit);
        let mut bytes = Vec::with_capacity(entry.size() as usize);
        entry.take(limit + 1).read_to_end(&mut bytes)?;
        anyhow::ensure!(bytes.len() as u64 <= limit, "release artifact is too large");
        value = Some(bytes);
    }
    value.ok_or_else(|| anyhow::anyhow!("release archive does not contain {expected}"))
}

fn component_dir() -> Option<PathBuf> {
    if let Some(value) = std::env::var_os("RC_COMPONENT_DIR").filter(|value| !value.is_empty()) {
        return Some(PathBuf::from(value));
    }
    if let Some(value) = std::env::var_os("RC_DATA_DIR").filter(|value| !value.is_empty()) {
        return Some(PathBuf::from(value).join("components"));
    }
    std::env::var_os("HOME")
        .map(PathBuf::from)
        .map(|home| home.join(".local/share/rc/components"))
}

fn write_executable(path: &Path, bytes: &[u8]) -> anyhow::Result<()> {
    let mut file = File::create(path)?;
    file.write_all(bytes)?;
    file.flush()?;
    file.sync_all()?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(path, fs::Permissions::from_mode(0o755))?;
    }
    Ok(())
}

fn atomic_write(path: &Path, bytes: &[u8]) -> anyhow::Result<()> {
    let parent = path
        .parent()
        .ok_or_else(|| anyhow::anyhow!("path has no parent"))?;
    fs::create_dir_all(parent)?;
    let temporary = parent.join(format!(
        ".{}.{}.tmp",
        path.file_name()
            .and_then(|value| value.to_str())
            .unwrap_or("rc"),
        std::process::id()
    ));
    fs::write(&temporary, bytes)?;
    replace_file(&temporary, path)
}

fn replace_file(source: &Path, destination: &Path) -> anyhow::Result<()> {
    fs::rename(source, destination).inspect_err(|_| {
        let _ = fs::remove_file(source);
    })?;
    Ok(())
}

#[cfg(test)]
mod tests;
