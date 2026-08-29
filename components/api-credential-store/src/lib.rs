wit_bindgen::generate!({ path: "../../wit", world: "api-credential-store", generate_all });

mod admin;
mod cli;
mod credentials;
mod crypto;
mod model;
mod storage;
mod validate;

use exports::ohrats::rc_api_credentials::credentials::Guest as CredentialsGuest;
use ohrats::rc_api_credentials::types::{
    CliAuthorization, Credential, Lifetime, Request, Scope, Verified,
};
use ohrats::rc_identity::types::HumanAuthorization;
use ohrats::rc_plugin::types::Service;

struct ApiCredentialStore;

impl Guest for ApiCredentialStore {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:api-credential-store".into(),
            version: "0.1.0".into(),
            provides: vec![service("ohrats:rc-api-credentials/credentials")],
            requires: vec![single("ohrats:rc-identity/admin-consumer")],
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

impl CredentialsGuest for ApiCredentialStore {
    fn create_api(
        a: HumanAuthorization,
        id: String,
        name: String,
        key: String,
        scopes: Vec<Scope>,
        life: Option<Lifetime>,
    ) -> Result<Credential, String> {
        credentials::create_api(a, id, name, key, scopes, life)
    }
    fn all(user_id: String) -> Result<Vec<Credential>, String> {
        credentials::list(&user_id)
    }
    fn get(id: String) -> Result<Option<Credential>, String> {
        credentials::get(&id)
    }
    fn revoke(a: HumanAuthorization, id: String) -> Result<bool, String> {
        credentials::revoke(a, &id)
    }
    fn verify(value: Request, now_ms: u64) -> Result<Verified, String> {
        credentials::verify(value, now_ms)
    }
    fn start_cli(
        client: String,
        key: String,
        life: Option<Lifetime>,
        request: String,
        device: String,
        user: String,
        now: u64,
    ) -> Result<CliAuthorization, String> {
        cli::start(client, key, life, request, device, user, now)
    }
    fn approve_cli(
        a: HumanAuthorization,
        request: String,
        user: String,
        browser_key: String,
    ) -> Result<Credential, String> {
        cli::approve(a, &request, &user, &browser_key)
    }
    fn poll_cli(request: String, device: String, now: u64) -> Result<Option<Credential>, String> {
        cli::poll(&request, &device, now)
    }
    fn revoke_cli(a: HumanAuthorization, id: String) -> Result<bool, String> {
        cli::revoke(a, &id)
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

fn single(name: &str) -> ohrats::rc_plugin::types::Requirement {
    ohrats::rc_plugin::types::Requirement {
        name: name.into(),
        version: "^0.1".into(),
        selection: ohrats::rc_plugin::types::Selection::Single,
    }
}
export!(ApiCredentialStore);
