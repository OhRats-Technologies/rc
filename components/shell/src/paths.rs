pub(crate) fn resolve(cwd: Option<&str>, value: &str, windows: bool) -> String {
    let Some(cwd) = cwd.filter(|cwd| *cwd != ".") else {
        return value.into();
    };
    if !windows {
        return if value.starts_with('/') {
            value.into()
        } else {
            join(cwd, value)
        };
    }
    if value.starts_with("\\\\")
        || value.starts_with("//")
        || drive(value).is_some_and(|_| value.as_bytes().get(2).is_some_and(is_separator))
    {
        return value.into();
    }
    if value.as_bytes().first().is_some_and(is_separator) {
        return drive(cwd)
            .map(|prefix| format!("{prefix}{value}"))
            .unwrap_or_else(|| value.into());
    }
    if let Some(prefix) = drive(value) {
        return if drive(cwd).is_some_and(|current| current.eq_ignore_ascii_case(prefix)) {
            join(cwd, &value[2..])
        } else {
            value.into()
        };
    }
    join(cwd, value)
}

fn drive(value: &str) -> Option<&str> {
    (value.len() >= 2 && value.as_bytes()[0].is_ascii_alphabetic() && value.as_bytes()[1] == b':')
        .then(|| &value[..2])
}

fn is_separator(value: &u8) -> bool {
    matches!(value, b'/' | b'\\')
}

fn join(cwd: &str, value: &str) -> String {
    if value.is_empty() {
        cwd.into()
    } else if cwd.ends_with(['/', '\\']) {
        format!("{cwd}{value}")
    } else {
        format!("{cwd}/{value}")
    }
}

#[cfg(test)]
mod tests {
    use super::resolve;

    #[test]
    fn resolves_unix_relative_paths() {
        assert_eq!(resolve(Some("/tmp/root"), "child", false), "/tmp/root/child");
        assert_eq!(resolve(Some("/tmp/root"), "/etc", false), "/etc");
    }

    #[test]
    fn resolves_windows_drive_unc_root_and_relative_paths() {
        assert_eq!(resolve(Some("C:\\root"), "child", true), "C:\\root/child");
        assert_eq!(resolve(Some("C:\\root"), "C:child", true), "C:\\root/child");
        assert_eq!(resolve(Some("C:\\root"), "D:child", true), "D:child");
        assert_eq!(resolve(Some("C:\\root"), "\\child", true), "C:\\child");
        assert_eq!(resolve(Some("C:\\root"), "D:\\child", true), "D:\\child");
        assert_eq!(resolve(Some("C:\\root"), "\\\\server\\share", true), "\\\\server\\share");
    }
}
