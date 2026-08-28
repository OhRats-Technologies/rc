use crate::{
    ohrats::rc_plugin::{artifact_cache, package_source::PackageArtifact},
    source,
    state::LockedComponent,
};

pub fn remember(artifact: &PackageArtifact) -> Result<(), String> {
    artifact_cache::write(&artifact.digest, &artifact.bytes)
}

pub fn exact(value: &LockedComponent) -> Result<Vec<u8>, String> {
    if let Some(bytes) = artifact_cache::read(&value.digest)? {
        return Ok(bytes);
    }
    let artifact = source::resolve_exact(&value.resolved_source)?;
    if artifact.digest != value.digest {
        return Err(format!(
            "locked digest {} is unavailable from {}; the source now resolves to {}",
            value.digest, value.resolved_source, artifact.digest
        ));
    }
    remember(&artifact)?;
    Ok(artifact.bytes)
}
