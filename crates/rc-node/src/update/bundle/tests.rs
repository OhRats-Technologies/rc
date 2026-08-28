use super::{CORE_COMPONENTS, extract_single, platform_target};
use flate2::{Compression, write::GzEncoder};
use tar::{Builder, Header};

#[test]
fn core_bundle_has_unique_names() {
    let mut names = CORE_COMPONENTS.to_vec();
    names.sort_unstable();
    names.dedup();
    assert_eq!(names.len(), CORE_COMPONENTS.len());
}

#[test]
fn kernel_host_updates_sibling_platform_binary() -> anyhow::Result<()> {
    assert_eq!(
        platform_target(std::path::Path::new("/opt/rc/rc-kernel"))?,
        std::path::Path::new("/opt/rc/rc")
    );
    assert_eq!(
        platform_target(std::path::Path::new("/opt/rc/rc"))?,
        std::path::Path::new("/opt/rc/rc")
    );
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
