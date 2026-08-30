use super::*;

#[cfg(any(target_os = "macos", target_os = "linux"))]
pub fn installed() -> bool {
    service_path().is_some_and(|path| path.exists())
}

#[cfg(windows)]
pub fn installed() -> bool {
    windows::installed()
}

#[cfg(not(any(target_os = "macos", target_os = "linux", windows)))]
pub fn installed() -> bool {
    false
}

#[cfg(target_os = "macos")]
pub fn install(executable: &Path, arguments: &[String], state_dir: &Path) -> Result<()> {
    install_launch_agent(executable, arguments, state_dir)
}

#[cfg(target_os = "linux")]
pub fn install(executable: &Path, arguments: &[String], _: &Path) -> Result<()> {
    install_systemd(executable, arguments)
}

#[cfg(windows)]
pub fn install(executable: &Path, arguments: &[String], _: &Path) -> Result<()> {
    windows::install(executable, arguments)
}

#[cfg(not(any(target_os = "macos", target_os = "linux", windows)))]
pub fn install(_: &Path, _: &[String], _: &Path) -> Result<()> {
    bail!("background service is not supported on this platform")
}

#[cfg(target_os = "macos")]
pub fn stop() -> Result<()> {
    if let Ok(path) = launch_agent_path() {
        let _ = Command::new("launchctl")
            .arg("bootout")
            .arg(format!("gui/{}", unsafe { libc::getuid() }))
            .arg(path)
            .status();
    }
    Ok(())
}

#[cfg(target_os = "linux")]
pub fn stop() -> Result<()> {
    let _ = Command::new("systemctl")
        .args(["--user", "stop", "rc.service"])
        .status();
    Ok(())
}

#[cfg(windows)]
pub fn stop() -> Result<()> {
    windows::stop()
}

#[cfg(not(any(target_os = "macos", target_os = "linux", windows)))]
pub fn stop() -> Result<()> {
    Ok(())
}

#[cfg(target_os = "macos")]
pub fn status() -> Result<()> {
    run(
        "launchctl",
        &[
            "print",
            &format!("gui/{}/{}", unsafe { libc::getuid() }, LABEL),
        ],
    )
}

#[cfg(target_os = "linux")]
pub fn status() -> Result<()> {
    run(
        "systemctl",
        &["--user", "status", "--no-pager", "rc.service"],
    )
}

#[cfg(windows)]
pub fn status() -> Result<()> {
    windows::status()
}

#[cfg(not(any(target_os = "macos", target_os = "linux", windows)))]
pub fn status() -> Result<()> {
    bail!("background service is not supported on this platform")
}

#[cfg(target_os = "macos")]
pub fn remove() -> Result<()> {
    remove_file(&launch_agent_path()?)
}

#[cfg(target_os = "linux")]
pub fn remove() -> Result<()> {
    let path = systemd_path()?;
    let _ = Command::new("systemctl")
        .args(["--user", "disable", "rc.service"])
        .status();
    remove_file(&path)?;
    let _ = Command::new("systemctl")
        .args(["--user", "daemon-reload"])
        .status();
    Ok(())
}

#[cfg(windows)]
pub fn remove() -> Result<()> {
    windows::remove()
}

#[cfg(not(any(target_os = "macos", target_os = "linux", windows)))]
pub fn remove() -> Result<()> {
    Ok(())
}

#[cfg(target_os = "macos")]
fn service_path() -> Option<PathBuf> {
    launch_agent_path().ok()
}

#[cfg(target_os = "linux")]
fn service_path() -> Option<PathBuf> {
    systemd_path().ok()
}
