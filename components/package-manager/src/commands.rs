use crate::{
    cache,
    ohrats::rc_plugin::component_store::{self, InstalledComponent},
    source::{self, ResolvedPackage},
    state::{DesiredComponent, DesiredState, LockState, LockedComponent},
};
#[path = "update_flow.rs"]
mod update_flow;
use std::collections::{BTreeMap, BTreeSet};

pub fn invoke(command: &str, args: &[String]) -> Result<u32, String> {
    match command {
        "add" => add(args),
        "remove" => remove(args),
        "install" => install(args),
        "list" => list(args),
        "outdated" => outdated(args),
        "update" => update(args),
        _ => Err(format!("unsupported command {command:?}")),
    }
}

fn add(args: &[String]) -> Result<u32, String> {
    let spec = one(args, "rc add <spec>")?;
    let resolved = source::resolve(spec, false)?;
    cache::remember(&resolved.artifact)?;
    let name = managed_name(&resolved);
    let installed = component_store::install(
        &name,
        &resolved.artifact.bytes,
        Some(&resolved.artifact.digest),
    )?;
    validate_catalog(&resolved, &installed)?;
    let mut desired = DesiredState::load()?;
    desired.components.insert(
        name.clone(),
        DesiredComponent {
            spec: spec.to_owned(),
        },
    );
    let mut lock = LockState::load()?;
    lock.replace(locked(&installed, spec, &resolved.artifact.source));
    desired.save()?;
    lock.save()?;
    println!("added {} {}", installed.id, installed.version);
    Ok(0)
}

fn remove(args: &[String]) -> Result<u32, String> {
    let name = one(args, "rc remove <name>")?;
    let mut desired = DesiredState::load()?;
    if !desired.components.contains_key(name) {
        return Err(format!("managed component {name:?} is not in rc.toml"));
    }
    if !component_store::remove(name)? {
        return Err(format!("managed component {name:?} is not installed"));
    }
    let mut lock = LockState::load()?;
    desired.components.remove(name);
    lock.remove(name);
    desired.save()?;
    lock.save()?;
    println!("removed {name}");
    Ok(0)
}

fn install(args: &[String]) -> Result<u32, String> {
    none(args, "rc install")?;
    let desired = DesiredState::load()?;
    let lock = LockState::load()?;
    validate_lock(&desired, &lock)?;
    let installed = installed_by_name()?;
    for value in &lock.component {
        if installed
            .get(&value.name)
            .is_some_and(|current| current.digest == value.digest)
        {
            continue;
        }
        let bytes = cache::exact(value)?;
        let actual = component_store::install(&value.name, &bytes, Some(&value.digest))?;
        validate_locked(value, &actual)?;
    }
    let desired_names = lock
        .component
        .iter()
        .map(|value| value.name.as_str())
        .collect::<BTreeSet<_>>();
    for value in installed.values() {
        if value.managed && !desired_names.contains(value.name.as_str()) {
            component_store::remove(&value.name)?;
        }
    }
    Ok(0)
}

fn list(args: &[String]) -> Result<u32, String> {
    none(args, "rc list")?;
    println!("NAME\tID\tVERSION\tMANAGED\tDIGEST");
    for value in component_store::installed()? {
        println!(
            "{}\t{}\t{}\t{}\t{}",
            value.name, value.id, value.version, value.managed, value.digest
        );
    }
    Ok(0)
}

fn outdated(args: &[String]) -> Result<u32, String> {
    let desired = DesiredState::load()?;
    let lock = LockState::load()?;
    println!("NAME\tCURRENT\tTARGET\tLATEST");
    for name in selected_names(&desired, args)? {
        let value = desired.components.get(&name).expect("selected component");
        let resolved = source::resolve(&value.spec, false)?;
        let current = lock
            .find(&name)
            .map(|item| item.version.as_str())
            .unwrap_or("-");
        let (target, latest) = versions(&resolved);
        let changed = lock
            .find(&name)
            .is_none_or(|item| item.digest != resolved.artifact.digest);
        if changed || target != latest {
            println!("{name}\t{current}\t{target}\t{latest}");
        }
    }
    Ok(0)
}

fn update(args: &[String]) -> Result<u32, String> {
    update_flow::run(args)
}

fn installed_by_name() -> Result<BTreeMap<String, InstalledComponent>, String> {
    Ok(component_store::installed()?
        .into_iter()
        .map(|value| (value.name.clone(), value))
        .collect())
}

fn validate_lock(desired: &DesiredState, lock: &LockState) -> Result<(), String> {
    let desired = desired.components.keys().collect::<BTreeSet<_>>();
    let locked = lock
        .component
        .iter()
        .map(|value| &value.name)
        .collect::<BTreeSet<_>>();
    if desired == locked {
        Ok(())
    } else {
        Err("rc.lock does not match rc.toml; run rc update".into())
    }
}

fn validate_locked(expected: &LockedComponent, actual: &InstalledComponent) -> Result<(), String> {
    if expected.id == actual.id
        && expected.version == actual.version
        && expected.digest == actual.digest
    {
        Ok(())
    } else {
        Err(format!(
            "locked component {} has unexpected metadata",
            expected.name
        ))
    }
}

fn validate_catalog(value: &ResolvedPackage, installed: &InstalledComponent) -> Result<(), String> {
    let Some(choice) = &value.catalog else {
        return Ok(());
    };
    let expected_id = format!("{}:{}", choice.namespace, choice.package);
    if installed.id != expected_id || installed.version != choice.target.to_string() {
        return Err(format!(
            "catalog selected {expected_id} {}, artifact contains {} {}",
            choice.target, installed.id, installed.version
        ));
    }
    Ok(())
}

fn managed_name(value: &ResolvedPackage) -> String {
    value
        .catalog
        .as_ref()
        .map(|choice| choice.package.clone())
        .unwrap_or_else(|| value.artifact.name.clone())
}

fn locked(value: &InstalledComponent, spec: &str, source: &str) -> LockedComponent {
    LockedComponent {
        name: value.name.clone(),
        id: value.id.clone(),
        version: value.version.clone(),
        spec: spec.into(),
        resolved_source: source.into(),
        digest: value.digest.clone(),
    }
}

fn versions(value: &ResolvedPackage) -> (String, String) {
    value.catalog.as_ref().map_or_else(
        || {
            (
                short_digest(&value.artifact.digest),
                short_digest(&value.artifact.digest),
            )
        },
        |choice| (choice.target.to_string(), choice.latest.to_string()),
    )
}

fn short_digest(value: &str) -> String {
    value.chars().take(19).collect()
}

fn selected_names(desired: &DesiredState, names: &[String]) -> Result<Vec<String>, String> {
    if names.is_empty() {
        return Ok(desired.components.keys().cloned().collect());
    }
    for name in names {
        if !desired.components.contains_key(name) {
            return Err(format!("unknown managed component {name:?}"));
        }
    }
    Ok(names.to_vec())
}

fn update_args(args: &[String]) -> Result<(Vec<String>, bool), String> {
    let mut names = Vec::new();
    let mut latest = false;
    for value in args {
        if value == "--latest" {
            latest = true;
        } else if value.starts_with('-') {
            return Err(format!("unknown update option {value:?}"));
        } else {
            names.push(value.clone());
        }
    }
    Ok((names, latest))
}

fn one<'a>(args: &'a [String], usage: &str) -> Result<&'a str, String> {
    match args {
        [value] => Ok(value),
        _ => Err(format!("usage: {usage}")),
    }
}

fn none(args: &[String], usage: &str) -> Result<(), String> {
    if args.is_empty() {
        Ok(())
    } else {
        Err(format!("usage: {usage}"))
    }
}
