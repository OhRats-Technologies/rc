wit_bindgen::generate!({
    path: "../../wit",
    world: "transport-test",
    generate_all,
});

use exports::ohrats::rc_transport::provider::Guest as ProviderGuest;
use ohrats::{
    rc_plugin::types::Service,
    rc_transport::types::{AnswerPlan, AnswerRequest},
};

struct TestTransport;

impl Guest for TestTransport {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:transport-test".into(),
            version: "0.1.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-transport/provider".into(),
                version: "0.1.0".into(),
                priority: 100,
                keys: vec!["test".into()],
            }],
            requires: Vec::new(),
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

impl ProviderGuest for TestTransport {
    fn plan_answer(transport: String, request: AnswerRequest) -> Result<AnswerPlan, String> {
        if transport != "test" {
            return Err("unsupported transport".into());
        }
        Ok(AnswerPlan {
            ice_servers: request.ice_servers,
            gather_timeout_ms: 100,
            connect_timeout_ms: 100,
        })
    }
}

export!(TestTransport);
