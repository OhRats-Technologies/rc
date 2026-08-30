use anyhow::{Context as _, Result};
use windows::Win32::{
    Foundation::HANDLE,
    System::Console::{
        CONSOLE_MODE, ENABLE_ECHO_INPUT, ENABLE_LINE_INPUT, ENABLE_PROCESSED_INPUT,
        ENABLE_PROCESSED_OUTPUT, ENABLE_VIRTUAL_TERMINAL_INPUT, ENABLE_VIRTUAL_TERMINAL_PROCESSING,
        GetConsoleMode, GetConsoleScreenBufferInfo, GetStdHandle, STD_INPUT_HANDLE,
        STD_OUTPUT_HANDLE, SetConsoleMode,
    },
};

pub fn attached() -> bool {
    input_handle().and_then(mode).is_ok()
}

pub fn size() -> Result<(u16, u16)> {
    let output = output_handle()?;
    let mut info = Default::default();
    unsafe { GetConsoleScreenBufferInfo(output, &mut info) }
        .context("read Windows terminal dimensions")?;
    let cols = i32::from(info.srWindow.Right) - i32::from(info.srWindow.Left) + 1;
    let rows = i32::from(info.srWindow.Bottom) - i32::from(info.srWindow.Top) + 1;
    Ok((
        u16::try_from(cols).unwrap_or(80).clamp(2, 500),
        u16::try_from(rows).unwrap_or(24).clamp(2, 500),
    ))
}

pub async fn next_resize(previous: (u16, u16)) -> (u16, u16) {
    let mut interval = tokio::time::interval(std::time::Duration::from_millis(150));
    interval.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Skip);
    loop {
        interval.tick().await;
        if let Ok(current) = size()
            && changed(previous, current)
        {
            return current;
        }
    }
}

fn changed(previous: (u16, u16), current: (u16, u16)) -> bool {
    previous != current
}

pub struct RawTerminal {
    input: HANDLE,
    input_mode: CONSOLE_MODE,
    output: HANDLE,
    output_mode: CONSOLE_MODE,
}

impl RawTerminal {
    pub fn enter() -> Result<Self> {
        let input = input_handle()?;
        let output = output_handle()?;
        let input_mode = mode(input)?;
        let output_mode = mode(output)?;
        let raw_input = (input_mode
            & !(ENABLE_ECHO_INPUT | ENABLE_LINE_INPUT | ENABLE_PROCESSED_INPUT))
            | ENABLE_VIRTUAL_TERMINAL_INPUT;
        let virtual_output =
            output_mode | ENABLE_PROCESSED_OUTPUT | ENABLE_VIRTUAL_TERMINAL_PROCESSING;
        unsafe { SetConsoleMode(input, raw_input) }.context("enable Windows terminal raw input")?;
        if let Err(error) = unsafe { SetConsoleMode(output, virtual_output) } {
            unsafe { SetConsoleMode(input, input_mode) }.ok();
            return Err(error).context("enable Windows virtual terminal output");
        }
        Ok(Self {
            input,
            input_mode,
            output,
            output_mode,
        })
    }
}

impl Drop for RawTerminal {
    fn drop(&mut self) {
        unsafe { SetConsoleMode(self.input, self.input_mode) }.ok();
        unsafe { SetConsoleMode(self.output, self.output_mode) }.ok();
    }
}

fn input_handle() -> Result<HANDLE> {
    unsafe { GetStdHandle(STD_INPUT_HANDLE) }.context("open Windows terminal input")
}

fn output_handle() -> Result<HANDLE> {
    unsafe { GetStdHandle(STD_OUTPUT_HANDLE) }.context("open Windows terminal output")
}

fn mode(handle: HANDLE) -> Result<CONSOLE_MODE> {
    let mut value = CONSOLE_MODE(0);
    unsafe { GetConsoleMode(handle, &mut value) }.context("read Windows terminal mode")?;
    Ok(value)
}

#[cfg(test)]
mod tests {
    use super::changed;

    #[test]
    fn resize_is_emitted_only_for_changed_dimensions() {
        assert!(!changed((80, 24), (80, 24)));
        assert!(changed((80, 24), (120, 40)));
    }
}
