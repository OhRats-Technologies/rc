#[cfg(not(windows))]
use std::{io, path::Path};

#[cfg(unix)]
pub fn protect_private_path(path: &Path, directory: bool) -> io::Result<()> {
    use std::os::unix::fs::PermissionsExt as _;
    let mode = if directory { 0o700 } else { 0o600 };
    std::fs::set_permissions(path, std::fs::Permissions::from_mode(mode))
}

#[cfg(unix)]
pub fn validate_private_path(path: &Path, directory: bool) -> io::Result<()> {
    use std::os::unix::fs::PermissionsExt as _;
    let metadata = std::fs::symlink_metadata(path)?;
    if metadata.file_type().is_symlink()
        || (directory && !metadata.is_dir())
        || (!directory && !metadata.is_file())
        || metadata.permissions().mode() & 0o077 != 0
    {
        return Err(io::Error::new(
            io::ErrorKind::PermissionDenied,
            "RC private path has unsafe permissions or type",
        ));
    }
    Ok(())
}

#[cfg(windows)]
mod windows_acl;

#[cfg(windows)]
pub use windows_acl::{protect_private_path, validate_private_path};

#[cfg(not(any(unix, windows)))]
pub fn protect_private_path(_: &Path, _: bool) -> io::Result<()> {
    Err(io::Error::new(
        io::ErrorKind::Unsupported,
        "private path protection is unsupported",
    ))
}

#[cfg(not(any(unix, windows)))]
pub fn validate_private_path(_: &Path, _: bool) -> io::Result<()> {
    Err(io::Error::new(
        io::ErrorKind::Unsupported,
        "private path validation is unsupported",
    ))
}
