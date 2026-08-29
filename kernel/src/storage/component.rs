use super::{fs, transaction};
use crate::{
    bindings::ohrats::rc_plugin::component_store::{InstalledComponent, PreparedSet},
    host::HostEnvironment,
};
use std::fs as std_fs;

pub(crate) fn installed(environment: &HostEnvironment) -> anyhow::Result<Vec<InstalledComponent>> {
    transaction::recover(environment)?;
    fs::installed(environment)
}

pub(crate) fn install(
    environment: &HostEnvironment,
    name: &str,
    artifact: &[u8],
    expected_digest: Option<&str>,
) -> anyhow::Result<InstalledComponent> {
    let prepared = transaction::prepare(
        environment,
        &[transaction::Candidate {
            name: name.into(),
            artifact: artifact.to_vec(),
            expected_digest: expected_digest.map(str::to_owned),
        }],
    )?;
    let prepared: PreparedSet = prepared.into();
    transaction::commit(environment, &prepared)?
        .into_iter()
        .next()
        .ok_or_else(|| anyhow::anyhow!("component install produced no component"))
}

pub(crate) fn remove(environment: &HostEnvironment, name: &str) -> anyhow::Result<bool> {
    transaction::recover(environment)?;
    fs::validate_token(name, "component name")?;
    let component = fs::component_path(environment, name);
    if !component.exists() {
        return Ok(false);
    }
    let marker = fs::marker_path(environment, name);
    anyhow::ensure!(
        marker.is_file(),
        "refusing to remove unmanaged component {name:?}"
    );
    std_fs::remove_file(component)?;
    match std_fs::remove_file(marker) {
        Ok(()) => {}
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
        Err(error) => return Err(error.into()),
    }
    Ok(true)
}
