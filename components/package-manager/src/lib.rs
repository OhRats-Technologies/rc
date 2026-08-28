mod cache;
mod catalog;
mod commands;
mod source;
mod state;

wit_bindgen::generate!({
    path: "../../wit",
    world: "package-manager",
});

use ohrats::rc_plugin::types::{Command, Requirement, Selection};

struct PackageManager;

impl Guest for PackageManager {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:package-manager".into(),
            version: "0.1.0".into(),
            provides: Vec::new(),
            requires: vec![Requirement {
                name: "ohrats:rc-plugin/package-source".into(),
                version: "^0.1".into(),
                selection: Selection::Keyed,
            }],
            commands: descriptors(),
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }

    fn deactivate() {}

    fn invoke(command: String, args: Vec<String>) -> Result<u32, String> {
        commands::invoke(&command, &args)
    }
}

fn descriptors() -> Vec<Command> {
    [
        (
            "add",
            "Add and install a managed component",
            "rc add <source>",
        ),
        ("remove", "Remove a managed component", "rc remove <name>"),
        (
            "install",
            "Install the desired locked component set",
            "rc install",
        ),
        ("list", "List installed components", "rc list"),
        (
            "outdated",
            "Show managed components whose source changed",
            "rc outdated [name...]",
        ),
        (
            "update",
            "Update managed components from their sources",
            "rc update [name...] [--latest]",
        ),
    ]
    .into_iter()
    .map(|(name, summary, usage)| Command {
        name: name.into(),
        summary: summary.into(),
        usage: usage.into(),
    })
    .collect()
}

export!(PackageManager);
