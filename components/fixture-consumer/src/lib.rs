wit_bindgen::generate!({
    path: "../../wit",
    world: "greeter-consumer",
});

use ohrats::rc_plugin::types::{Command, Requirement, Selection};

struct FixtureConsumer;

impl Guest for FixtureConsumer {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:fixture-consumer".into(),
            version: "1.0.0".into(),
            provides: Vec::new(),
            requires: vec![Requirement {
                name: "ohrats:rc-plugin/greeter".into(),
                version: "^0.1".into(),
                selection: Selection::Single,
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

    fn invoke(command: String, args: Vec<String>) -> Result<u32, String> {
        if command != "consume" {
            return Err(format!("unsupported command {command:?}"));
        }
        let name = args.first().map(String::as_str).unwrap_or("consumer");
        println!("{}", ohrats::rc_plugin::greeter::greet(name));
        Ok(0)
    }
}

export!(FixtureConsumer);
