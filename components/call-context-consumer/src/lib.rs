wit_bindgen::generate!({
    path: "../../wit",
    world: "greeter-consumer",
});

use ohrats::rc_plugin::types::{Command, Requirement, Selection};

struct CallContextConsumer;

impl Guest for CallContextConsumer {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:call-context-consumer".into(),
            version: "1.0.0".into(),
            provides: Vec::new(),
            requires: vec![Requirement {
                name: "ohrats:rc-plugin/greeter".into(),
                version: "^0.1".into(),
                selection: Selection::Single,
            }],
            commands: vec![Command {
                name: "caller-alt".into(),
                summary: "Report the immediate caller seen by a service provider".into(),
                usage: "rc caller-alt".into(),
            }],
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }

    fn deactivate() {}

    fn invoke(command: String, _args: Vec<String>) -> Result<u32, String> {
        if command != "caller-alt" {
            return Err(format!("unsupported command {command:?}"));
        }
        println!("{}", ohrats::rc_plugin::greeter::greet("__caller__"));
        Ok(0)
    }
}

export!(CallContextConsumer);
