wit_bindgen::generate!({
    path: "../../wit",
    world: "plugin",
});

use ohrats::rc_plugin::types::{Command, Requirement};

struct FixtureConsumer;

impl Guest for FixtureConsumer {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:fixture-consumer".into(),
            version: "1.0.0".into(),
            provides: Vec::new(),
            requires: vec![Requirement {
                name: "ohrats:test/greeter".into(),
                version: "^1".into(),
            }],
            commands: vec![Command {
                name: "consume".into(),
                summary: "Prove dependency-driven component activation".into(),
                usage: "rc consume".into(),
            }],
        }
    }

    fn activate() -> Result<(), String> {
        ohrats::rc_plugin::host::log(
            ohrats::rc_plugin::host::LogLevel::Info,
            "fixture consumer activated",
        );
        Ok(())
    }

    fn deactivate() {
        ohrats::rc_plugin::host::log(
            ohrats::rc_plugin::host::LogLevel::Info,
            "fixture consumer deactivated",
        );
    }

    fn invoke(command: String, _args: Vec<String>) -> Result<u32, String> {
        if command != "consume" {
            return Err(format!("unsupported command {command:?}"));
        }
        println!("consumer dependency is active");
        Ok(0)
    }
}

export!(FixtureConsumer);
