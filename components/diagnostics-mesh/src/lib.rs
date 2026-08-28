wit_bindgen::generate!({
    path: "../../wit",
    world: "diagnostics-mesh",
    generate_all,
});

use exports::ohrats::rc_mesh_diagnostics::reports::Guest as ReportsGuest;
use ohrats::{
    rc_diagnostics::{query, types::Event},
    rc_mesh_diagnostics::authorization,
    rc_plugin::types::{Requirement, Selection, Service},
};

struct DiagnosticsMesh;

impl Guest for DiagnosticsMesh {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:diagnostics-mesh".into(),
            version: "0.1.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-mesh-diagnostics/reports".into(),
                version: "0.1.0".into(),
                priority: 100,
                keys: Vec::new(),
            }],
            requires: vec![
                Requirement {
                    name: "ohrats:rc-diagnostics/query".into(),
                    version: "^0.1".into(),
                    selection: Selection::Single,
                },
                Requirement {
                    name: "ohrats:rc-mesh-diagnostics/authorization".into(),
                    version: "^0.1".into(),
                    selection: Selection::Single,
                },
            ],
            commands: Vec::new(),
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }

    fn deactivate() {}

    fn invoke(command: String, _args: Vec<String>) -> Result<u32, String> {
        Err(format!("unsupported command {command:?}"))
    }
}

impl ReportsGuest for DiagnosticsMesh {
    fn recent(peer_id: String, grant_id: String, limit: u32) -> Result<Vec<Event>, String> {
        if !authorization::allowed(&peer_id, &grant_id) {
            return Err("diagnostics grant is not authorized for this peer".into());
        }
        query::recent(limit)
    }
}

export!(DiagnosticsMesh);
