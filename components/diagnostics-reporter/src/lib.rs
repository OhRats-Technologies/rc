wit_bindgen::generate!({
    path: "../../wit",
    world: "diagnostics-reporter",
    generate_all,
});

use ohrats::{
    rc_diagnostics::{
        reporting,
        types::{Field, Level, Report},
    },
    rc_plugin::types::{Command, Requirement, Selection},
};

struct DiagnosticsReporter;

impl Guest for DiagnosticsReporter {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:diagnostics-reporter".into(),
            version: "0.1.0".into(),
            provides: Vec::new(),
            requires: vec![Requirement {
                name: "ohrats:rc-diagnostics/reporting".into(),
                version: "^0.1".into(),
                selection: Selection::Single,
            }],
            commands: vec![Command {
                name: "report".into(),
                summary: "Submit bounded diagnostic metadata".into(),
                usage: "rc report <code> <message>".into(),
            }],
        }
    }

    fn activate() -> Result<(), String> {
        reporting::submit(&Report {
            level: Level::Info,
            source: "rc.component".into(),
            code: "component.active".into(),
            message: "diagnostics reporter activated".into(),
            fields: vec![Field {
                name: "component".into(),
                value: "ohrats:diagnostics-reporter".into(),
            }],
        })?;
        Ok(())
    }

    fn deactivate() {}

    fn invoke(command: String, args: Vec<String>) -> Result<u32, String> {
        if command != "report" || args.len() < 2 {
            return Err("usage: rc report <code> <message>".into());
        }
        reporting::submit(&Report {
            level: Level::Info,
            source: "rc.cli".into(),
            code: args[0].clone(),
            message: args[1..].join(" "),
            fields: Vec::new(),
        })?;
        Ok(0)
    }
}

export!(DiagnosticsReporter);
