use crate::{
    ohrats::{
        rc_artifact_cache::{cache as provider_cache, types::FetchRequest},
        rc_plugin::{artifact_cache, package_source::PackageArtifact, service_registry},
    },
    source,
    state::LockedComponent,
};
use std::collections::BTreeSet;

const CACHE_SERVICE: &str = "ohrats:rc-artifact-cache/cache";
const CACHE_VERSION: &str = "^0.1";
const MAX_ARTIFACT_BYTES: u64 = 48 * 1024 * 1024;

pub fn remember(artifact: &PackageArtifact) -> Result<(), String> {
    artifact_cache::write(&artifact.digest, &artifact.bytes)
}

pub fn exact(value: &LockedComponent) -> Result<Vec<u8>, String> {
    if let Some(bytes) = provider(&value.digest)? {
        artifact_cache::write(&value.digest, &bytes)?;
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

fn provider(digest: &str) -> Result<Option<Vec<u8>>, String> {
    let mut seen = BTreeSet::new();
    for provider in service_registry::providers(CACHE_SERVICE, CACHE_VERSION)? {
        for key in provider.keys {
            if !seen.insert(key.clone()) {
                continue;
            }
            let request = FetchRequest {
                digest: digest.into(),
                max_bytes: MAX_ARTIFACT_BYTES,
            };
            let Some(artifact) = provider_cache::fetch(&key, &request)? else {
                continue;
            };
            if artifact.digest != digest {
                return Err(format!("cache provider {key:?} returned the wrong digest"));
            }
            return Ok(Some(artifact.bytes));
        }
    }
    Ok(None)
}
