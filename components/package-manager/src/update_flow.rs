use super::{locked, selected_names, update_args, validate_catalog};
use crate::{
    cache,
    ohrats::rc_plugin::component_store::{self, InstalledComponent, UpdateCandidate},
    source::{self, ResolvedPackage},
    state::{DesiredState, LockState},
};

struct Pending {
    name: String,
    spec: String,
    resolved: ResolvedPackage,
}

pub(super) fn run(args: &[String]) -> Result<u32, String> {
    let (names, use_latest) = update_args(args)?;
    let mut desired = DesiredState::load()?;
    let mut lock = LockState::load()?;
    let mut pending = Vec::new();
    for name in selected_names(&desired, &names)? {
        let original = desired
            .components
            .get(&name)
            .expect("selected component")
            .spec
            .clone();
        let resolved = source::resolve(&original, use_latest)?;
        cache::remember(&resolved.artifact)?;
        let spec = if use_latest {
            resolved
                .catalog
                .as_ref()
                .map_or(original.clone(), |choice| choice.updated_spec())
        } else {
            original
        };
        pending.push(Pending {
            name,
            spec,
            resolved,
        });
    }

    let prepared = component_store::prepare(
        &pending
            .iter()
            .map(|value| UpdateCandidate {
                name: value.name.clone(),
                artifact: value.resolved.artifact.bytes.clone(),
                expected_digest: Some(value.resolved.artifact.digest.clone()),
            })
            .collect::<Vec<_>>(),
    )?;
    for value in &pending {
        let prepared_value = prepared
            .components
            .iter()
            .find(|candidate| candidate.name == value.name)
            .map(|candidate| InstalledComponent {
                name: candidate.name.clone(),
                id: candidate.id.clone(),
                version: candidate.version.clone(),
                digest: candidate.digest.clone(),
                managed: true,
            })
            .ok_or_else(|| format!("prepared update omitted component {:?}", value.name));
        let prepared_value = match prepared_value {
            Ok(value) => value,
            Err(error) => {
                component_store::abort(&prepared);
                return Err(error);
            }
        };
        if let Err(error) = validate_catalog(&value.resolved, &prepared_value) {
            component_store::abort(&prepared);
            return Err(error);
        }
    }
    let installed = component_store::commit(&prepared)?;
    for value in pending {
        let actual = installed
            .iter()
            .find(|candidate| candidate.name == value.name)
            .ok_or_else(|| format!("committed update omitted component {:?}", value.name))?;
        let changed = lock
            .find(&value.name)
            .is_none_or(|item| item.digest != actual.digest);
        desired
            .components
            .get_mut(&value.name)
            .expect("selected component")
            .spec = value.spec.clone();
        lock.replace(locked(actual, &value.spec, &value.resolved.artifact.source));
        if changed {
            println!("updated {} {}", actual.id, actual.version);
        }
    }
    desired.save()?;
    lock.save()?;
    Ok(0)
}
