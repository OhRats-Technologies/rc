mod fixture;

use super::verify;
use crate::ohrats::rc_webauthn::types::UserHandleMode;

#[test]
fn verifies_registration_and_authentication() {
    let value = fixture::load();
    let registration = verify::registration(&value.algorithm, value.registration_request())
        .expect("registration fixture should verify");
    assert_eq!(registration.credential.algorithm, "es256");
    assert_eq!(registration.credential.sign_count, 0);
    assert_eq!(registration.aaguid, vec![1; 16]);
    assert!(registration.user_verified);

    let authentication = verify::authentication(
        &value.algorithm,
        value.authentication_request(registration.credential),
    )
    .expect("authentication fixture should verify");
    assert_eq!(authentication.credential.sign_count, 1);
    assert!(authentication.sign_count_advanced);
    assert!(authentication.user_verified);
}

#[test]
fn rejects_tampering_and_missing_discoverable_user_handles() {
    let value = fixture::load();
    let registration = verify::registration(&value.algorithm, value.registration_request())
        .expect("registration fixture should verify");
    let mut tampered = value.authentication_request(registration.credential.clone());
    tampered.signature[0] ^= 0x01;
    assert!(verify::authentication(&value.algorithm, tampered).is_err());

    let mut discoverable = value.authentication_request(registration.credential);
    discoverable.user_handle_mode = UserHandleMode::Discoverable;
    discoverable.response_user_handle = None;
    assert!(verify::authentication(&value.algorithm, discoverable).is_err());
}

#[test]
fn rejects_algorithm_and_origin_mismatches() {
    let value = fixture::load();
    assert!(verify::registration("rs256", value.registration_request()).is_err());
    let mut wrong_origin = value.registration_request();
    wrong_origin.relying_party.origin = "https://other.example.test".into();
    assert!(verify::registration(&value.algorithm, wrong_origin).is_err());
}
