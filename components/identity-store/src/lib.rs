wit_bindgen::generate!({
    path: "../../wit",
    world: "identity-store",
    generate_all,
});

mod ceremony;
mod session;
mod storage;
mod time;
mod user;
mod validate;

use exports::{
    ohrats::rc_identity::{ceremonies::Guest as CeremoniesGuest, users::Guest as UsersGuest},
    ohrats::rc_session::{lookup::Guest as LookupGuest, management::Guest as ManagementGuest},
};
use ohrats::{
    rc_identity::types::{Ceremony, User},
    rc_plugin::types::Service,
    rc_session::types::{IssuedSession, Session},
};

struct IdentityStore;

impl Guest for IdentityStore {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:identity-store".into(),
            version: "0.1.0".into(),
            provides: vec![
                service("ohrats:rc-identity/users"),
                service("ohrats:rc-identity/ceremonies"),
                service("ohrats:rc-session/lookup"),
                service("ohrats:rc-session/management"),
            ],
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

impl UsersGuest for IdentityStore {
    fn create(id: String, display_name: String) -> Result<User, String> {
        user::create(id, display_name)
    }

    fn get(id: String) -> Result<Option<User>, String> {
        user::get(&id)
    }

    fn count() -> Result<u64, String> {
        user::count()
    }
}

impl CeremoniesGuest for IdentityStore {
    fn put(value: Ceremony) -> Result<(), String> {
        ceremony::put(value)
    }

    fn take(id: String, kind: String) -> Result<Option<Ceremony>, String> {
        ceremony::take(&id, &kind)
    }
}

impl LookupGuest for IdentityStore {
    fn find(cookie_header: String) -> Result<Option<Session>, String> {
        session::find(&cookie_header)
    }
}

impl ManagementGuest for IdentityStore {
    fn issue(user_id: String, expires_at_ms: u64) -> Result<IssuedSession, String> {
        session::issue(user_id, expires_at_ms)
    }

    fn revoke(cookie_header: String) -> Result<bool, String> {
        session::revoke(&cookie_header)
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

export!(IdentityStore);
