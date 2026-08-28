wit_bindgen::generate!({
    path: "../../wit",
    world: "plugin",
});

use ohrats::rc_plugin::types::Command;

struct Collision;

impl Guest for Collision {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:fixture-collision".into(),
            version: "1.0.0".into(),
            provides: Vec::new(),
            requires: Vec::new(),
            commands: vec![Command {
                name: "hello".into(),
                summary: "Collide with the provider fixture".into(),
                usage: "rc hello".into(),
            }],
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }

    fn deactivate() {}

    fn invoke(_command: String, _args: Vec<String>) -> Result<u32, String> {
        Ok(0)
    }
}

export!(Collision);
