wit_bindgen::generate!({
    path: "../../wit",
    world: "http-stream-finite",
    generate_all,
});
use exports::ohrats::rc_http::handler::Guest as HttpGuest;
use ohrats::{
    rc_http::types::{Request, Response},
    rc_plugin::types::Service,
};
struct Finite;
impl Guest for Finite {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:http-stream-finite".into(),
            version: "0.1.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-http/handler".into(),
                version: "0.1.0".into(),
                priority: 100,
                keys: Vec::new(),
            }],
            requires: Vec::new(),
            commands: Vec::new(),
        }
    }
    fn activate() -> Result<(), String> {
        Ok(())
    }
    fn deactivate() {}
    fn invoke(_: String, _: Vec<String>) -> Result<u32, String> {
        Err("no commands".into())
    }
}
impl HttpGuest for Finite {
    fn handle(value: Request) -> Result<Option<Response>, String> {
        Ok((value.path == "/finite").then(|| Response {
            status: 200,
            headers: Vec::new(),
            body: b"finite".to_vec(),
        }))
    }
}
export!(Finite);
