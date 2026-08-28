wit_bindgen::generate!({ path: "../../wit", world: "ssh-policy-store", generate_all });

mod key;
mod policy;
mod store;

use exports::ohrats::rc_ssh::{
    credentials::Guest as CredentialsGuest, policy::Guest as PolicyGuest,
};
use ohrats::rc_plugin::types::Service;
use ohrats::rc_ssh::types::{PublicKey, SessionPolicy, SessionRequest};

struct SshPolicyStore;

impl Guest for SshPolicyStore {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:ssh-policy-store".into(),
            version: "0.1.0".into(),
            provides: vec![
                service("ohrats:rc-ssh/credentials"),
                service("ohrats:rc-ssh/policy"),
            ],
            requires: Vec::new(),
            commands: Vec::new(),
        }
    }
    fn activate() -> Result<(), String> {
        Ok(())
    }
    fn deactivate() {}
    fn invoke(command: String, _: Vec<String>) -> Result<u32, String> {
        Err(format!("unsupported command {command:?}"))
    }
}

impl CredentialsGuest for SshPolicyStore {
    fn register(
        user_id: String,
        control_client_id: String,
        name: String,
        public_key: String,
        created_at_ms: u64,
    ) -> Result<PublicKey, String> {
        key::register(user_id, control_client_id, name, public_key, created_at_ms)
    }
    fn list_keys(user_id: String) -> Result<Vec<PublicKey>, String> {
        key::list(&user_id)
    }
    fn get(id: String) -> Result<Option<PublicKey>, String> {
        key::get(&id)
    }
    fn revoke(id: String, user_id: String) -> Result<bool, String> {
        key::revoke(&id, &user_id)
    }
}

impl PolicyGuest for SshPolicyStore {
    fn authorize(request: SessionRequest) -> Result<SessionPolicy, String> {
        policy::authorize(request)
    }
    fn authorized_key_line(algorithm: String, key_data: String) -> Result<Option<String>, String> {
        policy::authorized_key_line(&algorithm, &key_data)
    }
}

fn service(name: &str) -> Service {
    Service {
        name: name.into(),
        version: "0.1.0".into(),
        priority: 100,
        keys: Vec::new(),
    }
}
export!(SshPolicyStore);
