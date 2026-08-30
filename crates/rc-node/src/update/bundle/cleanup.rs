use std::path::Path;

pub(super) fn versions(root: &Path, active: &Path, previous: Option<&Path>) -> anyhow::Result<()> {
    for entry in std::fs::read_dir(root)? {
        let entry = entry?;
        let kind = entry.file_type()?;
        let path = entry.path();
        if !kind.is_dir()
            || kind.is_symlink()
            || path == active
            || previous.is_some_and(|value| path == value)
            || entry
                .file_name()
                .to_str()
                .and_then(|value| semver::Version::parse(value).ok())
                .is_none()
        {
            continue;
        }
        std::fs::remove_dir_all(path)?;
    }
    Ok(())
}
