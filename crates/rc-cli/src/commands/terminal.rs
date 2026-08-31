use anyhow::{Result, bail};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_protocol::ControlMessage;
#[cfg(unix)]
use std::io;
#[cfg(unix)]
use std::os::fd::AsRawFd;

#[cfg(windows)]
mod windows_terminal;
use tokio::io::AsyncWriteExt;

pub(super) async fn wait_remote_process(
    sender: &crate::control_client::ControlSender,
    receiver: &mut crate::control_client::ControlReceiver,
    process_id: &str,
) -> Result<()> {
    let mut stdin_closed = false;
    loop {
        match receiver.recv().await? {
            ControlMessage::ProcessStarted { id } if id == process_id => {
                if !stdin_closed {
                    sender
                        .send(&ControlMessage::ProcessStdinClose { id })
                        .await?;
                    stdin_closed = true;
                }
            }
            ControlMessage::ProcessStdout { id, data } if id == process_id => {
                let mut stdout = tokio::io::stdout();
                stdout.write_all(&URL_SAFE_NO_PAD.decode(data)?).await?;
                stdout.flush().await?;
            }
            ControlMessage::ProcessStderr { id, data } if id == process_id => {
                let mut stderr = tokio::io::stderr();
                stderr.write_all(&URL_SAFE_NO_PAD.decode(data)?).await?;
                stderr.flush().await?;
            }
            ControlMessage::ProcessExit {
                id,
                exit_code,
                signal,
                error,
            } if id == process_id => {
                return process_exit(exit_code, &signal, &error);
            }
            _ => {}
        }
    }
}

pub(super) async fn read_shell_output(
    receiver: &mut crate::control_client::ControlReceiver,
    process_id: &str,
) -> Result<()> {
    let mut stdout = tokio::io::stdout();
    let mut stderr = tokio::io::stderr();
    loop {
        match receiver.recv().await? {
            ControlMessage::ProcessStdout { id, data } if id == process_id => {
                stdout.write_all(&URL_SAFE_NO_PAD.decode(data)?).await?;
                stdout.flush().await?;
            }
            ControlMessage::ProcessStderr { id, data } if id == process_id => {
                stderr.write_all(&URL_SAFE_NO_PAD.decode(data)?).await?;
                stderr.flush().await?;
            }
            ControlMessage::ProcessExit {
                id,
                exit_code,
                signal,
                error,
            } if id == process_id => {
                return process_exit(exit_code, &signal, &error);
            }
            _ => {}
        }
    }
}

fn process_exit(exit_code: Option<i32>, signal: &str, error: &str) -> Result<()> {
    let code = exit_code.unwrap_or(-1);
    if code == 0 {
        return Ok(());
    }
    if !error.is_empty() {
        bail!("{error} (process exited {code})");
    }
    if signal.is_empty() {
        bail!("process exited {code}");
    }
    bail!("process exited {code} ({signal})")
}

pub(super) fn terminal_attached() -> bool {
    #[cfg(unix)]
    unsafe {
        nix::libc::isatty(nix::libc::STDIN_FILENO) == 1
    }
    #[cfg(windows)]
    return windows_terminal::attached();
    #[cfg(not(any(unix, windows)))]
    false
}

pub(super) fn terminal_size() -> (u16, u16) {
    #[cfg(unix)]
    {
        let mut size = nix::libc::winsize {
            ws_row: 24,
            ws_col: 80,
            ws_xpixel: 0,
            ws_ypixel: 0,
        };
        unsafe { nix::libc::ioctl(io::stdin().as_raw_fd(), nix::libc::TIOCGWINSZ, &mut size) };
        (size.ws_col.clamp(2, 500), size.ws_row.clamp(2, 500))
    }
    #[cfg(windows)]
    return windows_terminal::size().unwrap_or((80, 24));
    #[cfg(not(any(unix, windows)))]
    (80, 24)
}

#[cfg(unix)]
pub(super) struct RawTerminal {
    original: nix::sys::termios::Termios,
}

#[cfg(unix)]
impl RawTerminal {
    pub(super) fn enter() -> Result<Self> {
        use nix::sys::termios::{SetArg, cfmakeraw, tcgetattr, tcsetattr};
        let stdin = io::stdin();
        let original = tcgetattr(&stdin)?;
        let mut raw = original.clone();
        cfmakeraw(&mut raw);
        tcsetattr(&stdin, SetArg::TCSANOW, &raw)?;
        Ok(Self { original })
    }
}

#[cfg(unix)]
impl Drop for RawTerminal {
    fn drop(&mut self) {
        use nix::sys::termios::{SetArg, tcsetattr};
        let stdin = io::stdin();
        let _ = tcsetattr(&stdin, SetArg::TCSANOW, &self.original);
    }
}

#[cfg(windows)]
pub(super) use windows_terminal::RawTerminal;

#[cfg(windows)]
pub(super) async fn next_terminal_resize(previous: (u16, u16)) -> (u16, u16) {
    windows_terminal::next_resize(previous).await
}

#[cfg(not(any(unix, windows)))]
pub(super) struct RawTerminal;

#[cfg(not(any(unix, windows)))]
impl RawTerminal {
    pub(super) fn enter() -> Result<Self> {
        Ok(Self)
    }
}
