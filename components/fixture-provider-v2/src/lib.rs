wit_bindgen::generate!({
    path: "../../wit",
    world: "greeter-provider",
});

use ohrats::rc_plugin::types::{Command, Service};

struct FixtureProviderV2;

impl Guest for FixtureProviderV2 {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:fixture-provider".into(),
            version: "2.0.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-plugin/greeter".into(),
                version: "0.1.0".into(),
                priority: 100,
                keys: Vec::new(),
            }],
            requires: Vec::new(),
            commands: vec![Command {
                name: "hello".into(),
                summary: "Print a v2 greeting".into(),
                usage: "rc hello [name]".into(),
            }],
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }

    fn deactivate() {}

    fn invoke(command: String, args: Vec<String>) -> Result<u32, String> {
        if command != "hello" {
            return Err(format!("unsupported command {command:?}"));
        }
        let name = args.first().map(String::as_str).unwrap_or("world");
        println!("hello from v2, {name}");
        Ok(0)
    }
}

impl exports::ohrats::rc_plugin::greeter::Guest for FixtureProviderV2 {
    fn greet(name: String) -> String {
        format!("hello from v2, {name}")
    }
}

export!(FixtureProviderV2);
