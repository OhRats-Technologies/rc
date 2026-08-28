use crate::{
    ohrats::rc_plugin::component_store,
    source,
    state::{DesiredComponent, DesiredState, LockState, LockedComponent},
};

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
    let source = one(args, "rc add <source>")?;
    let artifact = source::resolve(source)?;
    let installed =
        component_store::install(&artifact.name, &artifact.bytes, Some(&artifact.digest))?;
    let mut desired = DesiredState::load()?;
    desired.components.insert(
        installed.name.clone(),
        DesiredComponent {
            source: artifact.source.clone(),
        },
    );
    let mut lock = LockState::load()?;
    lock.replace(locked(&installed, artifact.source));
    desired.save()?;
    lock.save()?;
    println!("added {} {}", installed.id, installed.version);
    Ok(0)
}

fn remove(args: &[String]) -> Result<u32, String> {
    let name = one(args, "rc remove <name>")?;
    if !component_store::remove(name)? {
        return Err(format!("managed component {name:?} is not installed"));
    }
    let mut desired = DesiredState::load()?;
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
    sync(&[], false)
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
    println!("NAME\tCURRENT\tAVAILABLE");
    for (name, value) in selected(&desired, args)? {
        let artifact = source::resolve(&value.source)?;
        let current = lock
            .find(name)
            .map(|item| item.digest.as_str())
            .unwrap_or("-");
        if current != artifact.digest {
            println!("{name}\t{current}\t{}", artifact.digest);
        }
    }
    Ok(0)
}

fn update(args: &[String]) -> Result<u32, String> {
    sync(args, true)
}

fn sync(names: &[String], announce: bool) -> Result<u32, String> {
    let desired = DesiredState::load()?;
    let mut lock = LockState::load()?;
    for (name, value) in selected(&desired, names)? {
        let artifact = source::resolve(&value.source)?;
        if lock
            .find(name)
            .is_some_and(|item| item.digest == artifact.digest)
        {
            continue;
        }
        let installed = component_store::install(name, &artifact.bytes, Some(&artifact.digest))?;
        lock.replace(locked(&installed, artifact.source));
        if announce {
            println!("updated {} {}", installed.id, installed.version);
        }
    }
    lock.save()?;
    Ok(0)
}

fn selected<'a>(
    desired: &'a DesiredState,
    names: &[String],
) -> Result<Vec<(&'a String, &'a DesiredComponent)>, String> {
    if names.is_empty() {
        return Ok(desired.components.iter().collect());
    }
    names
        .iter()
        .map(|name| {
            desired
                .components
                .get_key_value(name)
                .ok_or_else(|| format!("unknown managed component {name:?}"))
        })
        .collect()
}

fn locked(value: &component_store::InstalledComponent, source: String) -> LockedComponent {
    LockedComponent {
        name: value.name.clone(),
        id: value.id.clone(),
        version: value.version.clone(),
        source,
        digest: value.digest.clone(),
    }
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
