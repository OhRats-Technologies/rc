use crate::now_ms;

pub(super) fn relative(value: Option<i64>) -> String {
    let Some(value) = value else {
        return "NEVER".into();
    };
    let seconds = ((now_ms() - value).max(0) / 1000) as u64;
    match seconds {
        0..=59 => "NOW".into(),
        60..=3_599 => format!("{}M AGO", seconds / 60),
        3_600..=86_399 => format!("{}H AGO", seconds / 3_600),
        86_400..=2_592_000 => format!("{}D AGO", seconds / 86_400),
        _ => format!("{}MO AGO", seconds / 2_592_000),
    }
}

pub(super) fn until(value: i64) -> String {
    if value == 0 {
        return "UNTIL REVOKED".into();
    }
    let seconds = ((value - now_ms()).max(0) / 1000) as u64;
    match seconds {
        0..=59 => "LESS THAN A MINUTE".into(),
        60..=3_599 => format!("{} MINUTES", seconds / 60),
        3_600..=86_399 => format!("{} HOURS", seconds / 3_600),
        _ => format!("{} DAYS", seconds / 86_400),
    }
}

pub(super) fn platform_icon(platform: &str) -> &'static str {
    match platform.to_ascii_lowercase().as_str() {
        "darwin" | "macos" => "icon-platform-macos",
        "linux" => "icon-platform-linux",
        "windows" | "win32" => "icon-platform-windows",
        _ => "icon-devices",
    }
}

pub(super) fn process_label(process: &serde_json::Value) -> String {
    let id = process
        .get("id")
        .and_then(serde_json::Value::as_str)
        .unwrap_or_default();
    let short = id.chars().take(8).collect::<String>();
    if process
        .get("terminal")
        .and_then(serde_json::Value::as_bool)
        .unwrap_or(false)
    {
        format!("Terminal {short}")
    } else {
        format!("Process {short}")
    }
}

pub(super) fn process_state(process: &serde_json::Value) -> String {
    let status = process
        .get("status")
        .and_then(serde_json::Value::as_str)
        .unwrap_or("unknown");
    match status {
        "exited" => process
            .get("exit_code")
            .and_then(serde_json::Value::as_i64)
            .map(|code| format!("EXIT {code}"))
            .unwrap_or_else(|| "EXITED".into()),
        value => value.to_ascii_uppercase(),
    }
}
