wit_bindgen::generate!({
    path: "../../wit",
    world: "identity-http",
    generate_all,
});

mod config;
mod flow;
mod pages;
mod request;
mod response;
mod time;
mod webauthn;

include!(concat!(env!("OUT_DIR"), "/assets.rs"));

use exports::ohrats::rc_http::handler::Guest as HttpGuest;
use ohrats::{
    rc_http::types::{Request, Response},
    rc_plugin::types::{Command, Requirement, Selection, Service},
};

struct IdentityHttp;

impl Guest for IdentityHttp {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:identity-http".into(),
            version: "0.1.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-http/handler".into(),
                version: "0.1.0".into(),
                priority: 220,
                keys: Vec::new(),
            }],
            requires: vec![
                single("ohrats:rc-identity/users"),
                single("ohrats:rc-identity/credentials"),
                single("ohrats:rc-identity/ceremonies"),
                single("ohrats:rc-session/lookup"),
                single("ohrats:rc-session/management"),
                keyed("ohrats:rc-webauthn/verifier"),
                single("ohrats:rc-webui/shell"),
            ],
            commands: vec![Command {
                name: "identity-config".into(),
                summary: "Read or change identity HTTP deployment configuration".into(),
                usage: "rc identity-config [public-url URL|auto|setup-token TOKEN|none]".into(),
            }],
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }

    fn deactivate() {}

    fn invoke(command: String, args: Vec<String>) -> Result<u32, String> {
        if command != "identity-config" {
            return Err(format!("unsupported command {command:?}"));
        }
        config::invoke(&args)
    }
}

impl HttpGuest for IdentityHttp {
    fn handle(value: Request) -> Result<Option<Response>, String> {
        flow::handle(value)
    }
}

fn single(name: &str) -> Requirement {
    Requirement {
        name: name.into(),
        version: "^0.1".into(),
        selection: Selection::Single,
    }
}

fn keyed(name: &str) -> Requirement {
    Requirement {
        name: name.into(),
        version: "^0.1".into(),
        selection: Selection::Keyed,
    }
}

export!(IdentityHttp);
