wit_bindgen::generate!({
    path: "../../wit",
    world: "identity-store",
    generate_all,
});

mod admin;
mod ceremony;
mod credential;
mod session;
mod storage;
mod time;
mod user;
mod validate;

use exports::{
    ohrats::rc_identity::{
        admin_consumer::Guest as AdminConsumerGuest,
        admin_issuer::{Challenge, Guest as AdminIssuerGuest},
        ceremonies::Guest as CeremoniesGuest,
        credentials::{Guest as CredentialsGuest, Passkey},
        users::Guest as UsersGuest,
    },
    ohrats::rc_session::{lookup::Guest as LookupGuest, management::Guest as ManagementGuest},
};
use ohrats::{
    rc_identity::types::{Ceremony, HumanAuthorization, User},
    rc_plugin::types::{Requirement, Selection, Service},
    rc_session::types::{IssuedSession, Session},
    rc_webauthn::types::StoredCredential,
};

struct IdentityStore;

impl Guest for IdentityStore {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:identity-store".into(),
            version: "0.2.0".into(),
            provides: vec![
                service("ohrats:rc-identity/users"),
                service("ohrats:rc-identity/credentials"),
                service("ohrats:rc-identity/ceremonies"),
                service("ohrats:rc-session/lookup"),
                service("ohrats:rc-session/management"),
                service("ohrats:rc-identity/admin-issuer"),
                service("ohrats:rc-identity/admin-consumer"),
            ],
            requires: vec![Requirement {
                name: "ohrats:rc-webauthn/verifier".into(),
                version: "^0.1".into(),
                selection: Selection::Keyed,
            }],
            commands: Vec::new(),
        }
    }

    fn activate() -> Result<(), String> {
        Ok(())
    }

    fn deactivate() {
        admin::withdraw();
    }

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

    fn all() -> Result<Vec<User>, String> {
        user::all()
    }

    fn count() -> Result<u64, String> {
        user::count()
    }

    fn rename(id: String, display_name: String) -> Result<User, String> {
        user::rename(&id, display_name)
    }
}

impl CredentialsGuest for IdentityStore {
    fn create_user(
        user_id: String,
        display_name: String,
        passkey_name: String,
        value: StoredCredential,
    ) -> Result<User, String> {
        credential::create_user(user_id, display_name, passkey_name, value)
    }

    fn add(user_id: String, name: String, value: StoredCredential) -> Result<Passkey, String> {
        credential::add(user_id, name, value)
    }

    fn get_by_credential_id(id: Vec<u8>) -> Result<Option<Passkey>, String> {
        credential::get_by_credential_id(&id)
    }

    fn all(user_id: Option<String>) -> Result<Vec<Passkey>, String> {
        credential::all(user_id.as_deref())
    }

    fn update(id: String, value: StoredCredential, used_at_ms: u64) -> Result<Passkey, String> {
        credential::update(&id, value, used_at_ms)
    }

    fn remove(id: String, user_id: String) -> Result<bool, String> {
        credential::remove(&id, &user_id)
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

impl AdminIssuerGuest for IdentityStore {
    fn begin(
        session_cookie: String,
        browser_client_id: String,
        operation: String,
        relying_party: ohrats::rc_webauthn::types::RelyingParty,
    ) -> Result<Challenge, String> {
        admin::begin(
            &session_cookie,
            &browser_client_id,
            &operation,
            relying_party,
        )
    }

    fn issue(
        session_cookie: String,
        browser_client_id: String,
        challenge_id: String,
        authentication: ohrats::rc_webauthn::types::AuthenticationRequest,
    ) -> Result<HumanAuthorization, String> {
        Ok(HumanAuthorization {
            token: admin::issue(
                &session_cookie,
                &browser_client_id,
                &challenge_id,
                authentication,
            )?,
        })
    }
}

impl AdminConsumerGuest for IdentityStore {
    fn consume(
        authorization: HumanAuthorization,
        operation: String,
    ) -> Result<exports::ohrats::rc_identity::admin_consumer::Claim, String> {
        admin::consume(&authorization.token, &operation)
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
