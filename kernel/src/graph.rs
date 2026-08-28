use crate::component::LoadedComponent;
use semver::Version;
use std::{
    collections::{BTreeMap, BTreeSet},
    fs,
    path::{Path, PathBuf},
};

pub type Services = BTreeMap<String, Vec<(i32, Version)>>;

pub fn component_paths(directory: &Path) -> anyhow::Result<Vec<PathBuf>> {
    let mut paths = fs::read_dir(directory)?
        .filter_map(Result::ok)
        .map(|entry| entry.path())
        .filter(|path| {
            path.extension()
                .is_some_and(|extension| extension == "wasm")
        })
        .collect::<Vec<_>>();
    paths.sort();
    Ok(paths)
}

pub fn available_services<'a>(
    components: impl IntoIterator<Item = (&'a str, &'a LoadedComponent)>,
    excluded: &BTreeSet<String>,
) -> Services {
    let mut services = Services::new();
    for (id, component) in components {
        if excluded.contains(id) || !component.is_active() {
            continue;
        }
        for service in &component.descriptor.provides {
            services
                .entry(service.name.clone())
                .or_default()
                .push((service.priority, service.version.clone()));
        }
    }
    services
}

pub fn requirements_met(component: &LoadedComponent, services: &Services) -> bool {
    component.descriptor.requires.iter().all(|requirement| {
        services.get(&requirement.name).is_some_and(|versions| {
            versions
                .iter()
                .any(|(_, version)| requirement.version.matches(version))
        })
    })
}
