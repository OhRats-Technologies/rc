use crate::ServiceCommand;
use anyhow::{Context, Result, bail};
use std::{
    fs,
    path::{Path, PathBuf},
    process::Command,
    thread,
    time::Duration,
};

const LABEL: &str = "party.ohrats.rc";

pub fn command(command: ServiceCommand) -> Result<()> {
    let dir = rc_node::resolve_state_dir(None);
    match command {
        ServiceCommand::Install => {
            rc_node::load_state(&dir)
                .context("enroll this machine before installing the RC service")?;
            install(&dir)
        }
        ServiceCommand::Start => {
            rc_node::load_state(&dir)
                .context("enroll this machine before starting the RC service")?;
            start(&dir)
        }
        ServiceCommand::Stop => stop(),
        ServiceCommand::Status => status(),
        ServiceCommand::Uninstall => remove(),
    }
}

pub fn install(state_dir: &Path) -> Result<()> {
    fs::create_dir_all(state_dir)?;
    let executable = std::env::current_exe()?;
    match std::env::consts::OS {
        "macos" => install_launch_agent(&executable, state_dir),
        "linux" => install_systemd(&executable, state_dir),
        other => bail!("background service is not supported on {other}"),
    }
}

pub fn installed() -> bool {
    service_path().is_some_and(|path| path.exists())
}

pub fn restart() -> Result<()> {
    let dir = rc_node::resolve_state_dir(None);
    match std::env::consts::OS {
        "macos" => start_launch_agent(&launch_agent_path()?),
        "linux" => run("systemctl", &["--user", "restart", "rc.service"]),
        _ => start(&dir),
    }
}

pub fn start(state_dir: &Path) -> Result<()> {
    match std::env::consts::OS {
        "macos" => {
            let path = launch_agent_path()?;
            if !path.exists() {
                return install(state_dir);
            }
            start_launch_agent(&path)
        }
        "linux" => run("systemctl", &["--user", "start", "rc.service"]),
        other => bail!("background service is not supported on {other}"),
    }
}

pub fn stop() -> Result<()> {
    match std::env::consts::OS {
        "macos" => {
            if let Ok(path) = launch_agent_path() {
                let _ = Command::new("launchctl")
                    .arg("bootout")
                    .arg(format!("gui/{}", unsafe { libc::getuid() }))
                    .arg(path)
                    .status();
            }
            Ok(())
        }
        "linux" => {
            let _ = Command::new("systemctl")
                .args(["--user", "stop", "rc.service"])
                .status();
            Ok(())
        }
        _ => Ok(()),
    }
}

pub fn status() -> Result<()> {
    match std::env::consts::OS {
        "macos" => run(
            "launchctl",
            &[
                "print",
                &format!("gui/{}/{}", unsafe { libc::getuid() }, LABEL),
            ],
        ),
        "linux" => run(
            "systemctl",
            &["--user", "status", "--no-pager", "rc.service"],
        ),
        other => bail!("background service is not supported on {other}"),
    }
}

pub fn remove() -> Result<()> {
    let _ = stop();
    match std::env::consts::OS {
        "macos" => remove_file(&launch_agent_path()?),
        "linux" => {
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
        _ => Ok(()),
    }
}

fn install_launch_agent(executable: &Path, state_dir: &Path) -> Result<()> {
    let path = launch_agent_path()?;
    let parent = path
        .parent()
        .context("RC launch agent path has no parent directory")?;
    fs::create_dir_all(parent)?;
    let log = state_dir.join("node.log");
    let escape = |value: &Path| xml(value.to_string_lossy().as_ref());
    let plist = format!(
        "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n<plist version=\"1.0\"><dict><key>Label</key><string>{LABEL}</string><key>ProgramArguments</key><array><string>{}</string><string>run</string><string>--state-dir</string><string>{}</string></array><key>RunAtLoad</key><true/><key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict><key>ThrottleInterval</key><integer>3</integer><key>StandardOutPath</key><string>{}</string><key>StandardErrorPath</key><string>{}</string></dict></plist>\n",
        escape(executable),
        escape(state_dir),
        escape(&log),
        escape(&log)
    );
    fs::write(&path, plist)?;
    let domain = format!("gui/{}", unsafe { libc::getuid() });
    let _ = Command::new("launchctl")
        .args(["bootout", &domain])
        .arg(&path)
        .output();
    start_launch_agent(&path)
}

fn start_launch_agent(path: &Path) -> Result<()> {
    let domain = format!("gui/{}", unsafe { libc::getuid() });
    let target = format!("{domain}/{LABEL}");
    let loaded = Command::new("launchctl")
        .args(["print", &target])
        .output()
        .is_ok_and(|output| output.status.success());
    if !loaded {
        run_owned(
            "launchctl",
            vec![
                "bootstrap".into(),
                domain,
                path.to_string_lossy().into_owned(),
            ],
        )?;
    }
    run("launchctl", &["kickstart", "-k", &target])?;
    thread::sleep(Duration::from_millis(750));
    let output = Command::new("launchctl")
        .args(["print", &target])
        .output()
        .context("could not verify RC launchd service")?;
    if !output.status.success()
        || !launch_agent_output_running(&String::from_utf8_lossy(&output.stdout))
    {
        bail!(
            "RC background service did not stay running; another `rc run` may be active. Stop it, then run `rc service start`"
        );
    }
    Ok(())
}

fn launch_agent_output_running(output: &str) -> bool {
    let mut running = false;
    let mut pid = false;
    for line in output.lines().map(str::trim) {
        running |= line == "state = running";
        pid |= line.starts_with("pid = ");
    }
    running && pid
}

fn install_systemd(executable: &Path, state_dir: &Path) -> Result<()> {
    if Command::new("systemctl").arg("--version").output().is_err() {
        bail!("systemd user services are unavailable; run `rc run` manually");
    }
    let path = systemd_path()?;
    let parent = path
        .parent()
        .context("RC systemd service path has no parent directory")?;
    fs::create_dir_all(parent)?;
    let unit = format!(
        "[Unit]\nDescription=RC Node\nAfter=network-online.target\n\n[Service]\nExecStart={} run --state-dir {}\nRestart=on-failure\nRestartSec=3\n\n[Install]\nWantedBy=default.target\n",
        quote(executable),
        quote(state_dir)
    );
    fs::write(&path, unit)?;
    run("systemctl", &["--user", "daemon-reload"])?;
    run("systemctl", &["--user", "enable", "--now", "rc.service"])
}

fn service_path() -> Option<PathBuf> {
    match std::env::consts::OS {
        "macos" => launch_agent_path().ok(),
        "linux" => systemd_path().ok(),
        _ => None,
    }
}
fn home() -> Result<PathBuf> {
    std::env::var_os("HOME")
        .map(PathBuf::from)
        .context("HOME is unavailable")
}
fn launch_agent_path() -> Result<PathBuf> {
    Ok(home()?
        .join("Library/LaunchAgents")
        .join(format!("{LABEL}.plist")))
}
fn systemd_path() -> Result<PathBuf> {
    Ok(home()?.join(".config/systemd/user/rc.service"))
}
fn remove_file(path: &Path) -> Result<()> {
    match fs::remove_file(path) {
        Ok(()) => Ok(()),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(()),
        Err(e) => Err(e.into()),
    }
}
fn xml(value: &str) -> String {
    value
        .replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
        .replace('"', "&quot;")
}
fn quote(path: &Path) -> String {
    format!(
        "\"{}\"",
        path.to_string_lossy()
            .replace('\\', "\\\\")
            .replace('"', "\\\"")
    )
}
fn run(name: &str, args: &[&str]) -> Result<()> {
    let status = Command::new(name).args(args).status()?;
    if !status.success() {
        bail!("{name} exited with {status}");
    }
    Ok(())
}
fn run_owned(name: &str, args: Vec<String>) -> Result<()> {
    let status = Command::new(name).args(args).status()?;
    if !status.success() {
        bail!("{name} exited with {status}");
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::launch_agent_output_running;

    #[test]
    fn launchd_state_requires_running_process() {
        assert!(launch_agent_output_running("state = running\npid = 123\n"));
        assert!(!launch_agent_output_running(
            "state = spawn scheduled\nlast exit code = 1\n"
        ));
        assert!(!launch_agent_output_running("state = running\n"));
    }
}
