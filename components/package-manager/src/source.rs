use crate::ohrats::rc_plugin::package_source::PackageArtifact;

pub fn parse(value: &str) -> Result<(String, String), String> {
    if value.starts_with('/') || value.starts_with("./") || value.starts_with("../") {
        return Ok(("file".into(), value.into()));
    }
    let (scheme, spec) = value
        .split_once(':')
        .ok_or_else(|| format!("package source {value:?} has no source prefix"))?;
    if scheme.is_empty() || spec.is_empty() {
        return Err(format!("invalid package source {value:?}"));
    }
    Ok((scheme.into(), spec.into()))
}

pub fn resolve(value: &str) -> Result<PackageArtifact, String> {
    let (scheme, spec) = parse(value)?;
    crate::ohrats::rc_plugin::package_source::resolve(&scheme, &spec)
}
