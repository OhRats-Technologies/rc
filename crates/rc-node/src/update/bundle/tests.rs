#[cfg(unix)]
use super::reject_kernel_downgrade;
use super::{
    CORE_COMPONENTS, core::LEGACY_CORE_COMPONENTS, core::extract_core, extract_single,
    platform_target,
};
use flate2::{Compression, write::GzEncoder};
use tar::{Builder, Header};

#[test]
fn core_bundle_has_unique_names() {
    let mut names = CORE_COMPONENTS.to_vec();
    names.sort_unstable();
    names.dedup();
    assert_eq!(names.len(), CORE_COMPONENTS.len());
    assert!(CORE_COMPONENTS.contains(&"scheduler"));
    assert!(CORE_COMPONENTS.contains(&"shell"));
}

#[test]
fn legacy_core_bundle_is_accepted() -> anyhow::Result<()> {
    let entries = LEGACY_CORE_COMPONENTS
        .iter()
        .map(|name| {
            (
                format!("components/{name}.wasm"),
                format!("legacy:{name}").into_bytes(),
            )
        })
        .collect::<Vec<_>>();
    let bundle = archive_owned(&entries)?;
    let values = extract_core(&bundle)?;
    assert_eq!(values.len(), LEGACY_CORE_COMPONENTS.len());
    Ok(())
}

#[test]
fn canonical_core_bundle_requires_matching_profile_lock() -> anyhow::Result<()> {
    use sha2::{Digest, Sha256};
    let mut entries = CORE_COMPONENTS
        .iter()
        .map(|name| {
            (
                format!("components/{name}.wasm"),
                format!("core:{name}").into_bytes(),
            )
        })
        .collect::<Vec<_>>();
    let mut lock = String::from("schema 1\nprofile ohrats:core\n");
    for (path, bytes) in &entries {
        let name = path
            .trim_start_matches("components/")
            .trim_end_matches(".wasm");
        lock.push_str(&format!(
            "component {name} sha256:{:x}\n",
            Sha256::digest(bytes)
        ));
    }
    entries.push(("profile.lock".into(), lock.into_bytes()));
    let values = extract_core(&archive_owned(&entries)?)?;
    assert_eq!(values.len(), CORE_COMPONENTS.len());
    entries.last_mut().unwrap().1.extend_from_slice(b"corrupt");
    assert!(extract_core(&archive_owned(&entries)?).is_err());
    Ok(())
}

#[test]
fn kernel_host_updates_sibling_platform_binary() -> anyhow::Result<()> {
    let directory = std::path::Path::new("opt").join("rc");
    let kernel = directory.join(rc_platform::executable_name("rc-kernel"));
    let controller = directory.join(rc_platform::executable_name("rc"));
    assert_eq!(platform_target(&kernel)?, controller);
    assert_eq!(platform_target(&controller)?, controller);
    Ok(())
}

#[test]
fn single_archive_rejects_extra_entries() -> anyhow::Result<()> {
    assert_eq!(
        extract_single(&archive(&[("rc", b"ok")])?, "rc", 10)?,
        b"ok"
    );
    assert!(extract_single(&archive(&[("rc", b"ok"), ("extra", b"bad")])?, "rc", 10).is_err());
    Ok(())
}

#[cfg(unix)]
#[test]
fn native_kernel_downgrades_are_rejected_independently_of_cli_version() -> anyhow::Result<()> {
    use std::os::unix::fs::PermissionsExt as _;

    let directory = std::env::temp_dir().join(format!(
        "rc-kernel-version-{}-{}",
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)?
            .as_nanos()
    ));
    std::fs::create_dir(&directory)?;
    let current = directory.join("rc-kernel");
    std::fs::write(&current, "#!/bin/sh\nprintf 'RC kernel 2.3.0\\n'\n")?;
    std::fs::set_permissions(&current, std::fs::Permissions::from_mode(0o755))?;
    assert!(reject_kernel_downgrade(&"2.2.9".parse()?, &current).is_err());
    reject_kernel_downgrade(&"2.3.0".parse()?, &current)?;
    reject_kernel_downgrade(&"2.4.0".parse()?, &current)?;
    std::fs::remove_dir_all(directory)?;
    Ok(())
}

#[test]
fn version_cleanup_retains_active_previous_and_unknown_directories() -> anyhow::Result<()> {
    let root = std::env::temp_dir().join(format!(
        "rc-version-cleanup-{}-{}",
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)?
            .as_nanos()
    ));
    let previous = root.join("1.0.0");
    let active = root.join("2.0.0");
    let stale = root.join("0.9.0");
    let unknown = root.join("locally-managed");
    for directory in [&previous, &active, &stale, &unknown] {
        std::fs::create_dir_all(directory)?;
    }
    super::cleanup::versions(&root, &active, Some(&previous))?;
    assert!(previous.is_dir());
    assert!(active.is_dir());
    assert!(!stale.exists());
    assert!(unknown.is_dir());
    std::fs::remove_dir_all(root)?;
    Ok(())
}

fn archive(entries: &[(&str, &[u8])]) -> anyhow::Result<Vec<u8>> {
    let encoder = GzEncoder::new(Vec::new(), Compression::default());
    let mut builder = Builder::new(encoder);
    for (name, contents) in entries {
        let mut header = Header::new_gnu();
        header.set_size(contents.len() as u64);
        header.set_mode(0o755);
        header.set_cksum();
        builder.append_data(&mut header, *name, *contents)?;
    }
    Ok(builder.into_inner()?.finish()?)
}

fn archive_owned(entries: &[(String, Vec<u8>)]) -> anyhow::Result<Vec<u8>> {
    let encoder = GzEncoder::new(Vec::new(), Compression::default());
    let mut builder = Builder::new(encoder);
    for (name, contents) in entries {
        let mut header = Header::new_gnu();
        header.set_size(contents.len() as u64);
        header.set_mode(0o644);
        header.set_cksum();
        builder.append_data(&mut header, name, contents.as_slice())?;
    }
    Ok(builder.into_inner()?.finish()?)
}
