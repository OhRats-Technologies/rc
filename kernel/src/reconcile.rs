use crate::{
    component::LoadedComponent,
    graph::{self, requirements_met},
    runtime::Entry,
};
use std::{
    collections::{BTreeMap, BTreeSet},
    path::PathBuf,
};

pub fn remove_missing(
    entries: &mut BTreeMap<String, Entry>,
    failed_paths: &mut BTreeMap<PathBuf, String>,
    desired: &BTreeSet<PathBuf>,
) -> bool {
    let removed_ids = entries
        .iter()
        .filter(|(_, entry)| {
            !desired.contains(&entry.current.path)
                && entry
                    .pending
                    .as_ref()
                    .is_none_or(|pending| !desired.contains(&pending.path))
        })
        .map(|(id, _)| id.clone())
        .collect::<Vec<_>>();
    if removed_ids.is_empty() {
        failed_paths.retain(|path, _| desired.contains(path));
        return false;
    }
    let removed = removed_ids.iter().cloned().collect::<BTreeSet<_>>();
    deactivate_unsatisfied(entries, &removed);
    for id in &removed_ids {
        entries.remove(id);
    }
    failed_paths.retain(|path, _| desired.contains(path));
    true
}

pub fn states(entries: &mut BTreeMap<String, Entry>) -> bool {
    let mut changed = false;
    let mut attempted = BTreeSet::new();
    loop {
        let services = services(entries, &BTreeSet::new());
        let active_commands = command_owners(entries);
        let mut progress = false;
        let ids = entries.keys().cloned().collect::<Vec<_>>();
        for id in ids {
            let entry = entries.get_mut(&id).expect("entry disappeared");
            let pending = activate_pending(entry, &id, &services, &active_commands);
            progress |= pending.progress;
            changed |= pending.changed;
            if entry.current.is_active() && !requirements_met(&entry.current, &services) {
                entry.current.deactivate();
                entry.error = None;
                progress = true;
                changed = true;
            } else if !entry.current.is_active()
                && requirements_met(&entry.current, &services)
                && attempted.insert(id.clone())
            {
                match command_conflict(&entry.current, &id, &active_commands) {
                    Some(error) => entry.error = Some(error),
                    None => match entry.current.activate() {
                        Ok(()) => {
                            entry.error = None;
                            progress = true;
                        }
                        Err(error) => {
                            entry.error = Some(format!("activation failed: {error:#}"));
                        }
                    },
                }
                changed = true;
            }
        }
        if !progress {
            break;
        }
    }
    changed
}

#[derive(Clone, Copy)]
struct Transition {
    changed: bool,
    progress: bool,
}

fn activate_pending(
    entry: &mut Entry,
    id: &str,
    services: &graph::Services,
    commands: &BTreeMap<String, String>,
) -> Transition {
    let Some(mut pending) = entry.pending.take() else {
        return Transition {
            changed: false,
            progress: false,
        };
    };
    if !requirements_met(&pending, services) {
        entry.pending = Some(pending);
        return Transition {
            changed: false,
            progress: false,
        };
    }
    if let Some(error) = command_conflict(&pending, id, commands) {
        entry.error = Some(format!("replacement activation failed: {error}"));
        entry.rejected_digest = Some(pending.digest.clone());
        return Transition {
            changed: true,
            progress: false,
        };
    }
    match pending.activate() {
        Ok(()) => {
            entry.current.deactivate();
            entry.current = pending;
            entry.error = None;
            entry.rejected_digest = None;
            Transition {
                changed: true,
                progress: true,
            }
        }
        Err(error) => {
            entry.error = Some(format!("replacement activation failed: {error:#}"));
            entry.rejected_digest = Some(pending.digest.clone());
            Transition {
                changed: true,
                progress: false,
            }
        }
    }
}

fn deactivate_unsatisfied(entries: &mut BTreeMap<String, Entry>, excluded: &BTreeSet<String>) {
    loop {
        let available = services(entries, excluded);
        let dependents = entries
            .iter()
            .filter(|(id, entry)| {
                !excluded.contains(*id)
                    && entry.current.is_active()
                    && !requirements_met(&entry.current, &available)
            })
            .map(|(id, _)| id.clone())
            .collect::<Vec<_>>();
        if dependents.is_empty() {
            break;
        }
        for id in dependents {
            entries
                .get_mut(&id)
                .expect("dependent disappeared")
                .current
                .deactivate();
        }
    }
}

fn services(entries: &BTreeMap<String, Entry>, excluded: &BTreeSet<String>) -> graph::Services {
    graph::available_services(
        entries
            .iter()
            .map(|(id, entry)| (id.as_str(), &entry.current)),
        excluded,
    )
}

fn command_owners(entries: &BTreeMap<String, Entry>) -> BTreeMap<String, String> {
    let mut commands = BTreeMap::new();
    for (id, entry) in entries {
        if !entry.current.is_active() {
            continue;
        }
        for command in &entry.current.descriptor.commands {
            commands.insert(command.name.clone(), id.clone());
        }
    }
    commands
}

fn command_conflict(
    component: &LoadedComponent,
    id: &str,
    commands: &BTreeMap<String, String>,
) -> Option<String> {
    component.descriptor.commands.iter().find_map(|command| {
        commands.get(&command.name).and_then(|owner| {
            (owner != id)
                .then(|| format!("command {:?} is already provided by {owner}", command.name))
        })
    })
}
