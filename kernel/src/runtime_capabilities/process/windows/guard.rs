use std::{ffi::OsString, os::windows::ffi::OsStrExt as _, process::Command};
use anyhow::Context as _;
use windows::{
    Win32::{
        Foundation::{CloseHandle, HANDLE, WAIT_OBJECT_0},
        System::{
            Console::SetConsoleCtrlHandler,
            Threading::{
                CreateEventW, INFINITE, OpenEventW, SYNCHRONIZATION_SYNCHRONIZE, SetEvent,
                WaitForSingleObject,
            },
        },
    },
    core::PCWSTR,
};

pub const MARKER: &str = "--rc-windows-execution-guard";

pub struct LaunchGate {
    handle: HANDLE,
    pub name: String,
}

impl LaunchGate {
    pub fn new() -> Result<Self, String> {
        let name = format!(
            "Local\\OhRats.RC.Execution.{}.{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap_or_default()
                .as_nanos()
        );
        let wide = wide(&name);
        let handle = unsafe { CreateEventW(None, true, false, PCWSTR(wide.as_ptr())) }
            .map_err(|error| error.to_string())?;
        Ok(Self { handle, name })
    }

    pub fn release(&self) -> Result<(), String> {
        unsafe { SetEvent(self.handle) }.map_err(|error| error.to_string())
    }
}

impl Drop for LaunchGate {
    fn drop(&mut self) {
        unsafe { CloseHandle(self.handle) }.ok();
    }
}

pub fn maybe_run() -> Option<anyhow::Result<()>> {
    let mut args = std::env::args_os();
    let _executable = args.next();
    if args.next().as_deref() != Some(std::ffi::OsStr::new(MARKER)) {
        return None;
    }
    Some(run(args.collect()))
}

fn run(args: Vec<OsString>) -> anyhow::Result<()> {
    let mut args = args.into_iter();
    let event = args
        .next()
        .ok_or_else(|| anyhow::anyhow!("execution guard event is missing"))?;
    let program = args
        .next()
        .ok_or_else(|| anyhow::anyhow!("execution guard program is missing"))?;
    let event = wide(&event);
    let handle = unsafe { OpenEventW(SYNCHRONIZATION_SYNCHRONIZE, false, PCWSTR(event.as_ptr())) }
        .context("open execution launch gate")?;
    let waited = unsafe { WaitForSingleObject(handle, INFINITE) };
    unsafe { CloseHandle(handle) }?;
    anyhow::ensure!(waited == WAIT_OBJECT_0, "execution guard wait failed");
    unsafe { SetConsoleCtrlHandler(Some(guard_control_event), true) }
        .context("install execution control handler")?;
    let status = Command::new(program)
        .args(args)
        .status()
        .context("start guarded execution target")?;
    std::process::exit(status.code().unwrap_or(1));
}

pub fn executable() -> Result<std::path::PathBuf, String> {
    let current = std::env::current_exe().map_err(|error| error.to_string())?;
    #[cfg(not(test))]
    return Ok(current);
    #[cfg(test)]
    {
        let debug = current
            .parent()
            .and_then(std::path::Path::parent)
            .ok_or_else(|| "Windows guard fixture directory is unavailable".to_owned())?;
        let fixture = debug.join("rc-windows-execution-guard-fixture.exe");
        fixture
            .is_file()
            .then_some(fixture)
            .ok_or_else(|| "Windows guard fixture was not built".to_owned())
    }
}

unsafe extern "system" fn guard_control_event(_: u32) -> windows::core::BOOL {
    true.into()
}

fn wide(value: impl AsRef<std::ffi::OsStr>) -> Vec<u16> {
    value.as_ref().encode_wide().chain(Some(0)).collect()
}
