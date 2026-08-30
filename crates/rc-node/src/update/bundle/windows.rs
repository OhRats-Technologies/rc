use super::cleanup;
use super::*;
use anyhow::Context as _;
use serde::{Deserialize, Serialize};

#[derive(Serialize, Deserialize)]
struct ActivationJournal {
    previous: Option<String>,
    names: Vec<String>,
}

pub(super) fn install(
    rc_archive: Option<&[u8]>,
    kernel_archive: &[u8],
    core_archive: &[u8],
    version: &str,
) -> anyhow::Result<()> {
    recover_interrupted()?;
    let kernel_name = rc_platform::executable_name("rc-kernel");
    let rc_name = rc_platform::executable_name("rc");
    let kernel = extract_single(kernel_archive, &kernel_name.to_string_lossy(), MAX_BINARY)?;
    let rc = rc_archive.map_or_else(read_current_cli, |archive| {
        extract_single(archive, &rc_name.to_string_lossy(), MAX_BINARY)
    })?;
    let components = extract_core(core_archive)?;
    let versions = rc_platform::runtime_versions_dir()?;
    let target = versions.join(version);
    let stage = versions.join(format!(".{version}.{}.new", std::process::id()));
    let _ = fs::remove_dir_all(&stage);
    fs::create_dir_all(&stage)?;
    let kernel_path = stage.join(&kernel_name);
    let rc_path = stage.join(&rc_name);
    write_executable(&kernel_path, &kernel)?;
    write_executable(&rc_path, &rc)?;
    let candidate_version =
        validate_kernel(&kernel_path, &components, &rc_platform::component_dir()?)?;
    if let Some(active) = rc_platform::active_runtime_dir() {
        reject_kernel_downgrade(&candidate_version, &active.join(&kernel_name))?;
    }
    validate_rc(&rc_path, version)?;
    let journal = prepare_rollback(&components.keys().cloned().collect::<Vec<_>>())?;
    let journal_path = journal_path()?;
    atomic_write(&journal_path, &serde_json::to_vec(&journal)?)?;
    if let Err(error) = activate(components, &stage, &target, &rc, &rc_name, &journal_path) {
        let rollback = recover_interrupted();
        return Err(error).context(format!("Windows activation rollback: {rollback:?}"));
    }
    atomic_write(
        &rc_platform::runtime_previous_activation_file()?,
        format!("{}\n", journal.previous.as_deref().unwrap_or_default()).as_bytes(),
    )?;
    if !journal_path.exists() {
        cleanup::versions(
            &versions,
            &target,
            journal.previous.as_deref().map(Path::new),
        )?;
    }
    Ok(())
}

fn read_current_cli() -> anyhow::Result<Vec<u8>> {
    let parent = std::env::current_exe()?
        .parent()
        .ok_or_else(|| anyhow::anyhow!("invalid executable path"))?
        .to_owned();
    Ok(fs::read(parent.join(rc_platform::executable_name("rc")))?)
}

fn activate(
    components: BTreeMap<String, Vec<u8>>,
    stage: &Path,
    target: &Path,
    rc: &[u8],
    rc_name: &std::ffi::OsStr,
    journal: &Path,
) -> anyhow::Result<()> {
    install_components(components)?;
    if target.exists() {
        fs::remove_dir_all(stage)?;
    } else {
        fs::rename(stage, target)?;
    }
    atomic_write(
        &rc_platform::runtime_activation_file()?,
        format!("{}\n", target.display()).as_bytes(),
    )?;
    let bin = rc_platform::binary_dir()?;
    fs::create_dir_all(&bin)?;
    let stable = bin.join(rc_name);
    let temporary = bin.join(format!(".rc-update-{}.exe", std::process::id()));
    write_executable(&temporary, rc)?;
    if !activate_cli(&temporary, &stable, journal)? {
        fs::remove_file(journal)?;
    }
    Ok(())
}

fn install_components(components: BTreeMap<String, Vec<u8>>) -> anyhow::Result<()> {
    let directory = rc_platform::component_dir()?;
    fs::create_dir_all(&directory)?;
    for (name, bytes) in components {
        install_core_component(&directory, &name, &bytes)?;
    }
    Ok(())
}

fn prepare_rollback(names: &[String]) -> anyhow::Result<ActivationJournal> {
    let backup = rollback_dir()?;
    let stage = backup.with_extension(format!("new-{}", std::process::id()));
    let _ = fs::remove_dir_all(&stage);
    fs::create_dir_all(stage.join("components"))?;
    let stable = rc_platform::binary_dir()?.join(rc_platform::executable_name("rc"));
    if stable.is_file() {
        fs::copy(stable, stage.join("rc.exe"))?;
    }
    let components = rc_platform::component_dir()?;
    for name in names {
        for suffix in ["wasm", "core"] {
            let source = components.join(format!("{name}.{suffix}"));
            if source.is_file() {
                fs::copy(
                    source,
                    stage.join("components").join(format!("{name}.{suffix}")),
                )?;
            }
        }
    }
    let _ = fs::remove_dir_all(&backup);
    fs::rename(stage, &backup)?;
    Ok(ActivationJournal {
        previous: fs::read_to_string(rc_platform::runtime_activation_file()?)
            .ok()
            .map(|value| value.trim().to_owned())
            .filter(|value| !value.is_empty()),
        names: names.to_vec(),
    })
}

fn recover_interrupted() -> anyhow::Result<()> {
    let path = journal_path()?;
    if !path.is_file() {
        return Ok(());
    }
    let journal: ActivationJournal = serde_json::from_slice(&fs::read(&path)?)?;
    let active = rc_platform::runtime_activation_file()?;
    match journal.previous {
        Some(previous) => atomic_write(&active, format!("{previous}\n").as_bytes())?,
        None if active.exists() => fs::remove_file(active)?,
        None => {}
    }
    let backup = rollback_dir()?;
    let stable = rc_platform::binary_dir()?.join(rc_platform::executable_name("rc"));
    let components = rc_platform::component_dir()?;
    for name in journal.names {
        for suffix in ["wasm", "core"] {
            restore_optional(
                &backup.join("components").join(format!("{name}.{suffix}")),
                &components.join(format!("{name}.{suffix}")),
            )?;
        }
    }
    if backup.join("rc.exe").is_file() {
        restore_cli(&backup.join("rc.exe"), &stable, &path)?;
    }
    fs::remove_file(path)?;
    Ok(())
}

fn restore_cli(source: &Path, destination: &Path, journal: &Path) -> anyhow::Result<()> {
    if destination.is_file() && fs::read(source)? == fs::read(destination)? {
        return Ok(());
    }
    let current = std::env::current_exe()?;
    let running =
        destination.exists() && current.canonicalize().ok() == destination.canonicalize().ok();
    if !running {
        return replace_copy(source, destination);
    }
    let temporary = destination.with_extension(format!("rollback-{}", std::process::id()));
    fs::copy(source, &temporary)?;
    spawn_post_exit(&temporary, destination, journal)?;
    anyhow::bail!("Windows CLI rollback will finish after this process exits; rerun rc upgrade")
}

fn restore_optional(source: &Path, destination: &Path) -> anyhow::Result<()> {
    if source.is_file() {
        return replace_copy(source, destination);
    }
    if destination.exists() {
        fs::remove_file(destination)?;
    }
    Ok(())
}

fn replace_copy(source: &Path, destination: &Path) -> anyhow::Result<()> {
    let temporary = destination.with_extension(format!("restore-{}", std::process::id()));
    fs::copy(source, &temporary)?;
    if destination.exists() {
        fs::remove_file(destination)?;
    }
    replace_file(&temporary, destination)
}

fn rollback_dir() -> anyhow::Result<PathBuf> {
    Ok(rc_platform::data_dir()?.join("runtime/upgrade-rollback"))
}

fn journal_path() -> anyhow::Result<PathBuf> {
    Ok(rc_platform::data_dir()?.join("runtime/upgrade-activation.json"))
}

fn activate_cli(source: &Path, destination: &Path, journal: &Path) -> anyhow::Result<bool> {
    let current = std::env::current_exe()?;
    let running =
        destination.exists() && current.canonicalize().ok() == destination.canonicalize().ok();
    if !running {
        if destination.exists() {
            fs::remove_file(destination)?;
        }
        replace_file(source, destination)?;
        return Ok(false);
    }
    spawn_post_exit(source, destination, journal)?;
    Ok(true)
}

fn spawn_post_exit(source: &Path, destination: &Path, journal: &Path) -> anyhow::Result<()> {
    let runtime = destination
        .parent()
        .ok_or_else(|| anyhow::anyhow!("RC binary path has no parent"))?;
    let helper = runtime.join(format!(".rc-activate-{}.ps1", std::process::id()));
    let script = r#"param([int]$ProcessToWait,[string]$Source,[string]$Destination,[string]$Journal)
$ErrorActionPreference='Stop'
Wait-Process -Id $ProcessToWait -ErrorAction SilentlyContinue
Move-Item -Force -LiteralPath $Source -Destination $Destination
Remove-Item -Force -LiteralPath $Journal
Remove-Item -Force -LiteralPath $MyInvocation.MyCommand.Path
"#;
    fs::write(&helper, script)?;
    Command::new("powershell.exe")
        .args(["-NoLogo", "-NoProfile", "-NonInteractive", "-File"])
        .arg(&helper)
        .arg(std::process::id().to_string())
        .arg(source)
        .arg(destination)
        .arg(journal)
        .spawn()
        .inspect_err(|_| {
            let _ = fs::remove_file(&helper);
            let _ = fs::remove_file(source);
        })?;
    Ok(())
}

#[cfg(test)]
#[path = "windows/tests.rs"]
mod tests;
