use std::{ffi::OsString, path::PathBuf};

mod security;
pub use security::{protect_private_path, validate_private_path};

#[derive(Debug, thiserror::Error)]
pub enum DirectoryError {
    #[error("the platform user directory is unavailable")]
    UserDirectoryUnavailable,
}

pub fn state_dir() -> Result<PathBuf, DirectoryError> {
    overridden("RC_STATE_DIR").map_or_else(default_state_dir, Ok)
}

pub fn data_dir() -> Result<PathBuf, DirectoryError> {
    overridden("RC_DATA_DIR").map_or_else(default_data_dir, Ok)
}

pub fn component_dir() -> Result<PathBuf, DirectoryError> {
    overridden("RC_COMPONENT_DIR").map_or_else(|| Ok(data_dir()?.join("components")), Ok)
}

pub fn wasmtime_cache_dir() -> Result<PathBuf, DirectoryError> {
    overridden("RC_WASMTIME_CACHE_DIR").map_or_else(|| Ok(data_dir()?.join("cache/wasmtime")), Ok)
}

pub fn binary_dir() -> Result<PathBuf, DirectoryError> {
    overridden("RC_INSTALL_BIN_DIR").map_or_else(default_binary_dir, Ok)
}

pub fn runtime_versions_dir() -> Result<PathBuf, DirectoryError> {
    Ok(data_dir()?.join("runtime/versions"))
}

pub fn runtime_activation_file() -> Result<PathBuf, DirectoryError> {
    Ok(data_dir()?.join("runtime/active"))
}

pub fn runtime_previous_activation_file() -> Result<PathBuf, DirectoryError> {
    Ok(data_dir()?.join("runtime/previous"))
}

pub fn active_runtime_dir() -> Option<PathBuf> {
    let file = runtime_activation_file().ok()?;
    let value = std::fs::read_to_string(file).ok()?;
    let value = value.trim();
    (!value.is_empty()).then(|| PathBuf::from(value))
}

pub fn user_home() -> Result<PathBuf, DirectoryError> {
    #[cfg(windows)]
    let name = "USERPROFILE";
    #[cfg(not(windows))]
    let name = "HOME";
    std::env::var_os(name)
        .filter(|value| !value.is_empty())
        .map(PathBuf::from)
        .ok_or(DirectoryError::UserDirectoryUnavailable)
}

pub fn executable_name(stem: &str) -> OsString {
    #[cfg(windows)]
    return format!("{stem}.exe").into();
    #[cfg(not(windows))]
    return stem.into();
}

fn overridden(name: &str) -> Option<PathBuf> {
    std::env::var_os(name)
        .filter(|value| !value.is_empty())
        .map(PathBuf::from)
}

#[cfg(windows)]
fn default_root() -> Result<PathBuf, DirectoryError> {
    default_root_from(std::env::var_os("LOCALAPPDATA"))
}

#[cfg(windows)]
fn default_root_from(value: Option<OsString>) -> Result<PathBuf, DirectoryError> {
    value
        .filter(|value| !value.is_empty())
        .map(PathBuf::from)
        .map(|path| path.join("OhRats/RC"))
        .ok_or(DirectoryError::UserDirectoryUnavailable)
}

#[cfg(not(windows))]
fn home() -> Result<PathBuf, DirectoryError> {
    user_home()
}

#[cfg(windows)]
fn default_state_dir() -> Result<PathBuf, DirectoryError> {
    Ok(default_root()?.join("state"))
}

#[cfg(not(windows))]
fn default_state_dir() -> Result<PathBuf, DirectoryError> {
    Ok(home()?.join(".config/rc"))
}

#[cfg(windows)]
fn default_data_dir() -> Result<PathBuf, DirectoryError> {
    Ok(default_root()?.join("data"))
}

#[cfg(not(windows))]
fn default_data_dir() -> Result<PathBuf, DirectoryError> {
    Ok(home()?.join(".local/share/rc"))
}

#[cfg(windows)]
fn default_binary_dir() -> Result<PathBuf, DirectoryError> {
    Ok(default_root()?.join("bin"))
}

#[cfg(not(windows))]
fn default_binary_dir() -> Result<PathBuf, DirectoryError> {
    Ok(home()?.join(".local/bin"))
}

#[cfg(test)]
mod tests {
    use super::executable_name;

    #[test]
    fn executable_uses_platform_suffix() {
        let expected = if cfg!(windows) { "rc.exe" } else { "rc" };
        assert_eq!(executable_name("rc"), expected);
    }

    #[cfg(windows)]
    #[test]
    fn windows_defaults_are_rooted_in_local_app_data() {
        let root = super::default_root_from(Some(r"C:\Users\RC User\AppData\Local".into()))
            .expect("Windows local application data");
        assert_eq!(
            root,
            std::path::Path::new(r"C:\Users\RC User\AppData\Local\OhRats\RC")
        );
    }

    #[cfg(windows)]
    #[test]
    fn windows_defaults_reject_missing_or_empty_local_app_data() {
        assert!(super::default_root_from(None).is_err());
        assert!(super::default_root_from(Some("".into())).is_err());
    }
}
