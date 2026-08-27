use rc_crypto::verify_webauthn_assertion;
use serde::Deserialize;

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct Vector {
    assertion: serde_json::Value,
    challenge: String,
    credential_id: String,
    origin: String,
    rp_id: String,
    stored_public_key: String,
}

#[test]
fn verifies_rs256_assertion_fixture() {
    let vector: Vector =
        serde_json::from_str(include_str!("../../../fixtures/webauthn-rs256.json")).unwrap();
    let assertion = serde_json::to_string(&vector.assertion).unwrap();
    verify_webauthn_assertion(
        &assertion,
        &vector.credential_id,
        &vector.stored_public_key,
        &vector.challenge,
        &vector.origin,
        &vector.rp_id,
    )
    .unwrap();
    assert!(
        verify_webauthn_assertion(
            &assertion,
            &vector.credential_id,
            &vector.stored_public_key,
            "wrong-challenge",
            &vector.origin,
            &vector.rp_id,
        )
        .is_err()
    );
}
