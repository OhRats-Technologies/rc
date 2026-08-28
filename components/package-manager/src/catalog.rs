use crate::ohrats::rc_plugin::catalog_store;
use semver::{Version, VersionReq};
use serde::Deserialize;

#[derive(Clone)]
pub struct CatalogChoice {
    pub namespace: String,
    pub package: String,
    pub target: Version,
    pub latest: Version,
    pub source: String,
}

impl CatalogChoice {
    pub fn updated_spec(&self) -> String {
        let range = if self.latest.major == 0 {
            format!("^0.{}", self.latest.minor)
        } else {
            format!("^{}", self.latest.major)
        };
        format!("{}:{}@{range}", self.namespace, self.package)
    }
}

#[derive(Deserialize)]
struct Catalog {
    schema: u32,
    namespace: String,
    #[serde(default)]
    package: Vec<CatalogPackage>,
}

#[derive(Deserialize)]
struct CatalogPackage {
    name: String,
    version: String,
    source: String,
}

pub fn resolve(namespace: &str, value: &str, use_latest: bool) -> Result<CatalogChoice, String> {
    validate_token(namespace, "catalog namespace")?;
    let (package, requested) = package_request(value)?;
    let bytes = catalog_store::read(namespace)?
        .ok_or_else(|| format!("catalog {namespace:?} is not installed"))?;
    let text = String::from_utf8(bytes).map_err(|error| error.to_string())?;
    let catalog: Catalog =
        toml::from_str(&text).map_err(|error| format!("invalid {namespace} catalog: {error}"))?;
    if catalog.schema != 1 || catalog.namespace != namespace {
        return Err(format!("invalid catalog identity for {namespace:?}"));
    }
    let mut releases = catalog
        .package
        .into_iter()
        .filter(|candidate| candidate.name == package)
        .map(|candidate| {
            Version::parse(&candidate.version)
                .map(|version| (version, candidate.source))
                .map_err(|error| format!("invalid {namespace}:{package} version: {error}"))
        })
        .collect::<Result<Vec<_>, _>>()?;
    releases.sort_by(|left, right| left.0.cmp(&right.0));
    let latest = releases
        .last()
        .map(|value| value.0.clone())
        .ok_or_else(|| format!("package {namespace}:{package} is not in the catalog"))?;
    let selected = if use_latest {
        releases.last()
    } else {
        releases
            .iter()
            .rev()
            .find(|(version, _)| requested.matches(version))
    }
    .ok_or_else(|| format!("no {namespace}:{package} release matches {requested}"))?;
    Ok(CatalogChoice {
        namespace: namespace.into(),
        package,
        target: selected.0.clone(),
        latest,
        source: selected.1.clone(),
    })
}

fn package_request(value: &str) -> Result<(String, VersionReq), String> {
    let (name, requirement) = value
        .rsplit_once('@')
        .map_or((value, "*"), |(name, requirement)| (name, requirement));
    validate_token(name, "package name")?;
    let requirement = VersionReq::parse(requirement)
        .map_err(|error| format!("invalid package version requirement: {error}"))?;
    Ok((name.into(), requirement))
}

fn validate_token(value: &str, label: &str) -> Result<(), String> {
    if !value.is_empty()
        && value.len() <= 96
        && value
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'_' | b'.'))
    {
        Ok(())
    } else {
        Err(format!("invalid {label} {value:?}"))
    }
}
