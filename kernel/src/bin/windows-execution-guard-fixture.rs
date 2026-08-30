#[cfg(windows)]
fn main() -> anyhow::Result<()> {
    use anyhow::Context as _;
    use std::{ffi::OsString, os::windows::ffi::OsStrExt as _, process::Command};
    use windows::{
        Win32::{
            Foundation::{CloseHandle, WAIT_OBJECT_0},
            System::{
                Console::SetConsoleCtrlHandler,
                Threading::{INFINITE, OpenEventW, SYNCHRONIZATION_SYNCHRONIZE, WaitForSingleObject},
            },
        },
        core::PCWSTR,
    };

    let mut args = std::env::args_os();
    anyhow::ensure!(
        args.next().is_some() && args.next().as_deref() == Some(std::ffi::OsStr::new("--rc-windows-execution-guard")),
        "fixture is only an execution-guard test target"
    );
    let event = args.next().context("execution guard event is missing")?;
    let program = args.next().context("execution guard program is missing")?;
    let wide: Vec<u16> = event.encode_wide().chain(Some(0)).collect();
    let handle = unsafe {
        OpenEventW(
            SYNCHRONIZATION_SYNCHRONIZE,
            false,
            PCWSTR(wide.as_ptr()),
        )
    }
    .context("open execution launch gate")?;
    let waited = unsafe { WaitForSingleObject(handle, INFINITE) };
    unsafe { CloseHandle(handle) }?;
    anyhow::ensure!(waited == WAIT_OBJECT_0, "execution guard wait failed");
    unsafe { SetConsoleCtrlHandler(Some(guard_control_event), true) }
        .context("install execution control handler")?;
    let status = Command::new(program).args(args.collect::<Vec<OsString>>()).status()?;
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
