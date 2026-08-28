wit_bindgen::generate!({
    path: "../../wit",
    world: "webauthn-es256",
    generate_all,
});

mod verify;

use exports::ohrats::rc_webauthn::verifier::Guest as VerifierGuest;
use ohrats::{
    rc_plugin::types::Service,
    rc_webauthn::types::{
        AuthenticationRequest, RegistrationRequest, VerifiedAuthentication, VerifiedRegistration,
    },
};

struct WebauthnEs256;

impl Guest for WebauthnEs256 {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:webauthn-es256".into(),
            version: "0.1.0".into(),
            provides: vec![Service {
                name: "ohrats:rc-webauthn/verifier".into(),
                version: "0.1.0".into(),
                priority: 100,
                keys: vec!["es256".into()],
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

impl VerifierGuest for WebauthnEs256 {
    fn verify_registration(
        algorithm: String,
        value: RegistrationRequest,
    ) -> Result<VerifiedRegistration, String> {
        verify::registration(&algorithm, value)
    }

    fn verify_authentication(
        algorithm: String,
        value: AuthenticationRequest,
    ) -> Result<VerifiedAuthentication, String> {
        verify::authentication(&algorithm, value)
    }
}

export!(WebauthnEs256);

#[cfg(test)]
mod tests;
