use super::{Group, NativeExit, Signal, display, native_exit};

impl Group {
    pub fn poll(&mut self, child: u32) -> Result<Option<NativeExit>, String> {
        let value = self
            .children
            .get_mut(&child)
            .ok_or("unknown native child")?;
        if self.process_group == i32::try_from(child).ok() {
            return leader(child);
        }
        value
            .try_wait()
            .map_err(display)
            .map(|value| value.map(native_exit))
    }
}

// Keep the leader waitable until group close. Reaping the last group member
// destroys the process group, so later shell stages cannot join it. Keeping
// this PID reserved also prevents a retained group from targeting a reused PID.
fn leader(pid: u32) -> Result<Option<NativeExit>, String> {
    let mut info: libc::siginfo_t = unsafe { std::mem::zeroed() };
    let result = unsafe {
        libc::waitid(
            libc::P_PID,
            pid as libc::id_t,
            &mut info,
            libc::WEXITED | libc::WNOHANG | libc::WNOWAIT,
        )
    };
    if result < 0 {
        let error = std::io::Error::last_os_error();
        if error.kind() == std::io::ErrorKind::Interrupted {
            return Ok(None);
        }
        return Err(error.to_string());
    }
    if unsafe { info.si_pid() } == 0 {
        return Ok(None);
    }
    let status = unsafe { info.si_status() };
    Ok(Some(NativeExit {
        code: (info.si_code == libc::CLD_EXITED).then_some(status as u32),
        signal: if info.si_code == libc::CLD_EXITED {
            None
        } else {
            match status {
                libc::SIGINT => Some(Signal::Interrupt),
                libc::SIGTERM => Some(Signal::Terminate),
                libc::SIGKILL => Some(Signal::Kill),
                _ => None,
            }
        },
    }))
}
