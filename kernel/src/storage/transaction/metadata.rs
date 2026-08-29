use super::{recovery, storage_fs};
use crate::bindings::ohrats::rc_plugin::component_store::PreparedComponent;
use std::{fs, path::Path};

pub(super) fn verify_manifest(
    directory: &Path,
    id: &str,
    components: &[PreparedComponent],
) -> anyhow::Result<()> {
    anyhow::ensure!(directory.is_dir(), "prepared update is no longer available");
    let expected = components
        .iter()
        .map(|value| value.name.as_str())
        .collect::<Vec<_>>();
    anyhow::ensure!(
        recovery::read_names(&directory.join("manifest"))? == expected,
        "prepared update manifest changed"
    );
    anyhow::ensure!(
        fs::read_to_string(directory.join("metadata"))? == metadata_text(id, components)?,
        "prepared update metadata changed"
    );
    for value in components {
        anyhow::ensure!(
            directory.join(format!("{}.wasm", value.name)).is_file(),
            "prepared artifact is missing"
        );
    }
    Ok(())
}

pub(super) fn write_metadata(
    path: &Path,
    id: &str,
    components: &[PreparedComponent],
) -> anyhow::Result<()> {
    storage_fs::atomic_write(path, metadata_text(id, components)?.as_bytes())
}

fn metadata_text(id: &str, components: &[PreparedComponent]) -> anyhow::Result<String> {
    storage_fs::validate_token(id, "transaction id")?;
    let mut text = format!("transaction-id\t{id}\n");
    for value in components {
        for (field, label) in [
            (&value.name, "component name"),
            (&value.id, "component id"),
            (&value.version, "component version"),
            (&value.digest, "component digest"),
        ] {
            anyhow::ensure!(
                !field.is_empty()
                    && !field
                        .bytes()
                        .any(|byte| matches!(byte, b'\n' | b'\r' | b'\t')),
                "invalid {label} in prepared update"
            );
        }
        text.push_str(&format!(
            "{}\t{}\t{}\t{}\n",
            value.name, value.id, value.version, value.digest
        ));
    }
    Ok(text)
}
