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
const MAX_COMPONENT: u64 = 48 << 20;

pub const CORE_COMPONENTS: &[&str] = &[
    "diagnostics-cli",
    "diagnostics-reporter",
    "diagnostics-store",
    "github-source",
    "http-source",
    "local-source",
    "oci-source",
    "package-manager",
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
    let bin_dir = executable
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
        replace_file(&replacement, &executable)?;
    }
    Ok(())
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

fn extract_core(archive: &[u8]) -> anyhow::Result<BTreeMap<String, Vec<u8>>> {
    let mut tar = tar::Archive::new(GzDecoder::new(archive));
    let mut values = BTreeMap::new();
    for entry in tar.entries()? {
        let entry = entry?;
        let path = entry.path()?.into_owned();
        if entry.header().entry_type().is_dir() && path.as_path() == Path::new("components") {
            continue;
        }
        anyhow::ensure!(
            entry.header().entry_type().is_file(),
            "invalid core component archive"
        );
        let text = path
            .to_str()
            .and_then(|value| value.strip_prefix("components/"))
            .and_then(|value| value.strip_suffix(".wasm"))
            .ok_or_else(|| anyhow::anyhow!("unexpected core component path {}", path.display()))?
            .to_owned();
        anyhow::ensure!(
            CORE_COMPONENTS.contains(&text.as_str()),
            "unexpected core component {text:?}"
        );
        anyhow::ensure!(
            !values.contains_key(&text),
            "duplicate core component {text:?}"
        );
        anyhow::ensure!(entry.size() <= MAX_COMPONENT, "core component is too large");
        let mut bytes = Vec::with_capacity(entry.size() as usize);
        entry.take(MAX_COMPONENT + 1).read_to_end(&mut bytes)?;
        anyhow::ensure!(
            bytes.len() as u64 <= MAX_COMPONENT,
            "core component is too large"
        );
        values.insert(text, bytes);
    }
    anyhow::ensure!(
        CORE_COMPONENTS
            .iter()
            .all(|name| values.contains_key(*name)),
        "core component archive is incomplete"
    );
    Ok(values)
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
mod tests {
    use super::{CORE_COMPONENTS, extract_single};
    use flate2::{Compression, write::GzEncoder};
    use tar::{Builder, Header};

    #[test]
    fn core_bundle_has_unique_names() {
        let mut names = CORE_COMPONENTS.to_vec();
        names.sort_unstable();
        names.dedup();
        assert_eq!(names.len(), CORE_COMPONENTS.len());
    }

    #[test]
    fn single_archive_rejects_extra_entries() -> anyhow::Result<()> {
        assert_eq!(
            extract_single(&archive(&[("rc", b"ok")])?, "rc", 10)?,
            b"ok"
        );
        assert!(extract_single(&archive(&[("rc", b"ok"), ("extra", b"bad")])?, "rc", 10).is_err());
        Ok(())
    }

    fn archive(entries: &[(&str, &[u8])]) -> anyhow::Result<Vec<u8>> {
        let encoder = GzEncoder::new(Vec::new(), Compression::default());
        let mut builder = Builder::new(encoder);
        for (name, contents) in entries {
            let mut header = Header::new_gnu();
            header.set_size(contents.len() as u64);
            header.set_mode(0o755);
            header.set_cksum();
            builder.append_data(&mut header, *name, *contents)?;
        }
        Ok(builder.into_inner()?.finish()?)
    }
}
