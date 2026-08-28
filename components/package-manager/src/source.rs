use crate::{
    catalog::{self, CatalogChoice},
    ohrats::rc_plugin::{
        package_source::{self, PackageArtifact},
        service_registry,
    },
};
use std::collections::BTreeSet;

const SOURCE_SERVICE: &str = "ohrats:rc-plugin/package-source";
const SOURCE_VERSION: &str = "^0.1";

pub struct ResolvedPackage {
    pub artifact: PackageArtifact,
    pub catalog: Option<CatalogChoice>,
}

pub fn resolve(value: &str, latest: bool) -> Result<ResolvedPackage, String> {
    let schemes = schemes()?;
    match classify(value, &schemes)? {
        SourceSpec::Direct { scheme, spec } => Ok(ResolvedPackage {
            artifact: package_source::resolve(&scheme, &spec)?,
            catalog: None,
        }),
        SourceSpec::Catalog { namespace, package } => {
            let choice = catalog::resolve(&namespace, &package, latest)?;
            let (scheme, spec) = direct(&choice.source, &schemes)?;
            Ok(ResolvedPackage {
                artifact: package_source::resolve(&scheme, &spec)?,
                catalog: Some(choice),
            })
        }
    }
}

pub fn resolve_exact(value: &str) -> Result<PackageArtifact, String> {
    let schemes = schemes()?;
    let (scheme, spec) = direct(value, &schemes)?;
    package_source::resolve(&scheme, &spec)
}

fn schemes() -> Result<BTreeSet<String>, String> {
    Ok(service_registry::providers(SOURCE_SERVICE, SOURCE_VERSION)?
        .into_iter()
        .flat_map(|provider| provider.keys)
        .collect())
}

enum SourceSpec {
    Direct { scheme: String, spec: String },
    Catalog { namespace: String, package: String },
}

fn classify(value: &str, schemes: &BTreeSet<String>) -> Result<SourceSpec, String> {
    if is_path(value) {
        return Ok(SourceSpec::Direct {
            scheme: "file".into(),
            spec: value.into(),
        });
    }
    if let Some((scheme, _)) = value.split_once("://") {
        ensure_scheme(scheme, schemes)?;
        return Ok(SourceSpec::Direct {
            scheme: scheme.into(),
            spec: value.into(),
        });
    }
    if let Some((prefix, rest)) = value.split_once(':') {
        if schemes.contains(prefix) {
            if rest.is_empty() {
                return Err(format!("invalid package source {value:?}"));
            }
            return Ok(SourceSpec::Direct {
                scheme: prefix.into(),
                spec: rest.into(),
            });
        }
        if !catalog_package(rest) {
            ensure_scheme(prefix, schemes)?;
        }
        return Ok(SourceSpec::Catalog {
            namespace: prefix.into(),
            package: rest.into(),
        });
    }
    Ok(SourceSpec::Catalog {
        namespace: "ohrats".into(),
        package: value.into(),
    })
}

fn direct(value: &str, schemes: &BTreeSet<String>) -> Result<(String, String), String> {
    match classify(value, schemes)? {
        SourceSpec::Direct { scheme, spec } => Ok((scheme, spec)),
        SourceSpec::Catalog { namespace, .. } => Err(format!(
            "catalog entries must resolve to a direct source; {namespace:?} is not a source provider"
        )),
    }
}

fn ensure_scheme(scheme: &str, schemes: &BTreeSet<String>) -> Result<(), String> {
    if schemes.contains(scheme) {
        Ok(())
    } else {
        Err(format!(
            "package source provider {scheme:?} is not installed"
        ))
    }
}

fn is_path(value: &str) -> bool {
    value.starts_with('/') || value.starts_with("./") || value.starts_with("../")
}

fn catalog_package(value: &str) -> bool {
    let name = value.rsplit_once('@').map_or(value, |(name, _)| name);
    !name.is_empty()
        && name.len() <= 96
        && name
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'_' | b'.'))
}

#[cfg(test)]
mod tests {
    use super::catalog_package;

    #[test]
    fn distinguishes_catalog_packages_from_source_payloads() {
        assert!(catalog_package("webui@^2"));
        assert!(!catalog_package("owner/repository"));
        assert!(!catalog_package("../plugin"));
    }
}
