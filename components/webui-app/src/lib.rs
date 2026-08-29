wit_bindgen::generate!({
    path: "../../wit",
    world: "webui-app",
    generate_all,
});

mod routes;

use exports::ohrats::rc_http::handler::Guest as HttpGuest;
use ohrats::rc_http::types::{Request, Response};
use ohrats::rc_plugin::types::{Requirement, Selection, Service};

struct WebUiApp;

impl Guest for WebUiApp {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:webui-app".into(),
            version: "0.1.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-http/handler".into(),
                version: "0.1.0".into(),
                priority: 90,
                keys: Vec::new(),
            }],
            requires: vec![
                requirement("ohrats:rc-session/lookup"),
                requirement("ohrats:rc-webui/shell"),
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

impl HttpGuest for WebUiApp {
    fn handle(value: Request) -> Result<Option<Response>, String> {
        routes::handle(value)
    }
}

fn requirement(name: &str) -> Requirement {
    Requirement {
        name: name.into(),
        version: "^0.1".into(),
        selection: Selection::Single,
    }
}

export!(WebUiApp);
