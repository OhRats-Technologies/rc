#[cfg(windows)]
fn main() -> anyhow::Result<()> {
    use anyhow::Context as _;
    use std::{ffi::OsString, os::windows::ffi::OsStrExt as _, process::Command};
    use windows::{
        Win32::{
            Foundation::{CloseHandle, WAIT_OBJECT_0},
            System::{
                Console::SetConsoleCtrlHandler,
                Threading::{
                    EVENT_MODIFY_STATE, INFINITE, OpenEventW, SYNCHRONIZATION_SYNCHRONIZE,
                    SetEvent, WaitForSingleObject,
                },
            },
        },
        core::PCWSTR,
    };

    let mut args = std::env::args_os();
    anyhow::ensure!(args.next().is_some(), "fixture executable is missing");
    let mode = args.next();
    if mode.as_deref() == Some(std::ffi::OsStr::new("--echo-argv")) {
        let values = args
            .map(|value| value.to_string_lossy().into_owned())
            .collect::<Vec<_>>();
        println!("{}", serde_json::to_string(&values)?);
        return Ok(());
    }
    anyhow::ensure!(
        mode.as_deref() == Some(std::ffi::OsStr::new("--rc-windows-execution-guard")),
        "fixture is only an execution-guard test target"
    );
    let event = args.next().context("execution guard event is missing")?;
    let ready = args
        .next()
        .context("execution guard ready event is missing")?;
    let program = args.next().context("execution guard program is missing")?;
    eprintln!("RC_GUARD_STAGE parsed target={program:?}");
    let wide: Vec<u16> = event.encode_wide().chain(Some(0)).collect();
    let handle = unsafe { OpenEventW(SYNCHRONIZATION_SYNCHRONIZE, false, PCWSTR(wide.as_ptr())) }
        .context("open execution launch gate")?;
    let ready_wide: Vec<u16> = ready.encode_wide().chain(Some(0)).collect();
    let ready_handle =
        unsafe { OpenEventW(EVENT_MODIFY_STATE, false, PCWSTR(ready_wide.as_ptr())) }
            .context("open execution ready event")?;
    unsafe { SetEvent(ready_handle) }.context("signal execution guard readiness")?;
    unsafe { CloseHandle(ready_handle) }?;
    let waited = unsafe { WaitForSingleObject(handle, INFINITE) };
    unsafe { CloseHandle(handle) }?;
    anyhow::ensure!(waited == WAIT_OBJECT_0, "execution guard wait failed");
    eprintln!("RC_GUARD_STAGE released");
    unsafe { SetConsoleCtrlHandler(Some(guard_control_event), true) }
        .context("install execution control handler")?;
    let status = Command::new(program)
        .args(args.collect::<Vec<OsString>>())
        .status()?;
    eprintln!("RC_GUARD_STAGE target-exit={status}");
    std::process::exit(status.code().unwrap_or(1));
}

#[cfg(windows)]
unsafe extern "system" fn guard_control_event(_: u32) -> windows::core::BOOL {
    true.into()
}

#[cfg(not(windows))]
fn main() {
    eprintln!("Windows execution guard fixture is unavailable on this platform");
    std::process::exit(1);
}
