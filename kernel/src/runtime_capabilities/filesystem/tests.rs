use super::{EntryKind, entry_kind, link_like, remove_path};
use std::fs;

#[cfg(unix)]
#[test]
fn unix_directory_symlink_is_link_like_and_not_a_directory_entry() -> anyhow::Result<()> {
    use std::os::unix::fs::symlink;
    let root = tempfile::tempdir()?;
    let target = root.path().join("target");
    let link = root.path().join("link");
    fs::create_dir(&target)?;
    fs::write(target.join("kept"), b"value")?;
    symlink(&target, &link)?;
    let metadata = fs::symlink_metadata(&link)?;
    assert!(link_like(&metadata));
    assert_eq!(entry_kind(&metadata), EntryKind::Symlink);
    remove_path(&link, true).map_err(anyhow::Error::msg)?;
    assert!(target.join("kept").is_file());
    Ok(())
}

#[cfg(windows)]
#[test]
fn windows_directory_reparse_link_is_not_traversed() -> anyhow::Result<()> {
    let root = tempfile::tempdir()?;
    let target = root.path().join("target");
    let link = root.path().join("link");
    fs::create_dir(&target)?;
    fs::write(target.join("kept"), b"value")?;
    let source = format!("mklink /J \"{}\" \"{}\"", link.display(), target.display());
    anyhow::ensure!(
        std::process::Command::new("cmd.exe")
            .args(["/D", "/C", &source])
            .status()?
            .success(),
        "could not create Windows directory junction"
    );
    let metadata = fs::symlink_metadata(&link)?;
    assert!(link_like(&metadata));
    assert_eq!(entry_kind(&metadata), EntryKind::Symlink);
    remove_path(&link, true).map_err(anyhow::Error::msg)?;
    assert!(target.join("kept").is_file());
    Ok(())
}
