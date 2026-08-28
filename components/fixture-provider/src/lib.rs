wit_bindgen::generate!({
    path: "../../wit",
    world: "plugin",
});

use ohrats::rc_plugin::types::{Command, Service};

struct FixtureProvider;

impl Guest for FixtureProvider {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:fixture-provider".into(),
            version: "1.0.0".into(),
            provides: vec![Service {
                name: "ohrats:test/greeter".into(),
                version: "1.0.0".into(),
                priority: 100,
            }],
            requires: Vec::new(),
            commands: vec![Command {
                name: "hello".into(),
                summary: "Print a greeting from a dynamically loaded component".into(),
                usage: "rc hello [name]".into(),
            }],
        }
    }

    fn activate() -> Result<(), String> {
        ohrats::rc_plugin::host::log(
            ohrats::rc_plugin::host::LogLevel::Info,
            "fixture provider activated",
        );
        Ok(())
    }

    fn deactivate() {
        ohrats::rc_plugin::host::log(
            ohrats::rc_plugin::host::LogLevel::Info,
            "fixture provider deactivated",
        );
    }

    fn invoke(command: String, args: Vec<String>) -> Result<u32, String> {
        if command != "hello" {
            return Err(format!("unsupported command {command:?}"));
        }
        let name = args.first().map(String::as_str).unwrap_or("world");
        println!("hello, {name}");
        Ok(0)
    }
}

export!(FixtureProvider);
