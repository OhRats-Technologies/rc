wit_bindgen::generate!({
    path: "../../wit",
    world: "updater",
    generate_all,
});

mod policy;

use ohrats::rc_plugin::types::Command;

struct Updater;

impl Guest for Updater {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:updater".into(),
            version: "0.1.0".into(),
            provides: Vec::new(),
            requires: Vec::new(),
            commands: vec![Command {
                name: "upgrade".into(),
                summary: "Replace the native kernel from a pinned artifact".into(),
                usage: "rc upgrade EXPECTED-DIGEST [--reexec]".into(),
            }],
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }

    fn deactivate() {}

    fn invoke(command: String, args: Vec<String>) -> Result<u32, String> {
        if command != "upgrade" {
            return Err(format!("unsupported command {command:?}"));
        }
        policy::upgrade(&args)
    }
}

export!(Updater);
