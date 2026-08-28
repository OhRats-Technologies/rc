wit_bindgen::generate!({
    path: "../../wit",
    world: "plugin",
});

use ohrats::rc_plugin::types::{Command, Service};

struct BrokenProvider;

impl Guest for BrokenProvider {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:fixture-provider".into(),
            version: "2.0.0".into(),
            provides: vec![Service {
                name: "ohrats:test/greeter".into(),
                version: "2.0.0".into(),
                priority: 100,
            }],
            requires: Vec::new(),
            commands: vec![Command {
                name: "hello".into(),
                summary: "Broken replacement fixture".into(),
                usage: "rc hello".into(),
            }],
        }
    }

    fn activate() -> Result<(), String> {
        Err("intentional activation failure".into())
    }

    fn deactivate() {}

    fn invoke(_command: String, _args: Vec<String>) -> Result<u32, String> {
        Err("broken fixture must never be invoked".into())
    }
}

export!(BrokenProvider);
