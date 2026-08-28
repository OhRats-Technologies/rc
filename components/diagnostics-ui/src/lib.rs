wit_bindgen::generate!({
    path: "../../wit",
    world: "diagnostics-ui",
    generate_all,
});

use ohrats::{
    rc_diagnostics::query,
    rc_plugin::types::{Requirement, Selection},
    rc_webui::{slots, types::Page},
};

const PAGE_ID: &str = "diagnostics";

struct DiagnosticsUi;

impl Guest for DiagnosticsUi {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:diagnostics-ui".into(),
            version: "0.1.0".into(),
            provides: Vec::new(),
            requires: vec![
                Requirement {
                    name: "ohrats:rc-diagnostics/query".into(),
                    version: "^0.1".into(),
                    selection: Selection::Single,
                },
                Requirement {
                    name: "ohrats:rc-webui/slots".into(),
                    version: "^0.1".into(),
                    selection: Selection::Single,
                },
            ],
            commands: Vec::new(),
        }
    }

    fn activate() -> Result<(), String> {
        let health = query::status();
        slots::register_page(&Page {
            id: PAGE_ID.into(),
            title: "Diagnostics".into(),
            path: "/diagnostics".into(),
            summary: "Bounded component health and error metadata".into(),
            content: format!(
                "Retained: {}\nWarnings: {}\nErrors: {}\nNewest sequence: {}",
                health.retained, health.warnings, health.errors, health.newest_sequence
            ),
        })
    }

    fn deactivate() {
        let _ = slots::remove_page(PAGE_ID);
    }

    fn invoke(command: String, _args: Vec<String>) -> Result<u32, String> {
        Err(format!("unsupported command {command:?}"))
    }
}

export!(DiagnosticsUi);
