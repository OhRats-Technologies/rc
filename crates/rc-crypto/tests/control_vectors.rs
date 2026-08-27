use rc_crypto::*;
use serde::Deserialize;

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct Vector {
    challenge: String,
    device_id: String,
    client_id: String,
    session_id: String,
    sequence: u64,
    client_private: String,
    client_public: String,
    node_static_private: String,
    node_static_public: String,
    node_ephemeral_private: String,
    node_ephemeral_public: String,
    shared_static: String,
    shared_ephemeral: String,
    key_hex: String,
    nonce_c2_n: String,
    nonce_n2_c: String,
    aad_c2_n: String,
    plaintext: String,
    ciphertext: String,
    session_payload: String,
    ready_payload: String,
    node_identity_seed: String,
    node_identity_public: String,
    ready_signature: String,
}

#[test]
fn matches_control_crypto_fixture() {
    let vector: Vector =
        serde_json::from_str(include_str!("../../../fixtures/control-crypto.json")).unwrap();
    assert_eq!(
        x25519_public(&vector.client_private).unwrap(),
        vector.client_public
    );
    assert_eq!(
        x25519_public(&vector.node_static_private).unwrap(),
        vector.node_static_public
    );
    assert_eq!(
        x25519_public(&vector.node_ephemeral_private).unwrap(),
        vector.node_ephemeral_public
    );
    assert_eq!(
        base64url(&x25519_shared(&vector.client_private, &vector.node_static_public).unwrap()),
        vector.shared_static
    );
    assert_eq!(
        base64url(&x25519_shared(&vector.client_private, &vector.node_ephemeral_public).unwrap()),
        vector.shared_ephemeral
    );
    let key = derive_client_key(
        &vector.client_private,
        &vector.node_static_public,
        &vector.node_ephemeral_public,
        &vector.challenge,
        &vector.device_id,
        &vector.client_id,
    )
    .unwrap();
    assert_eq!(hex(&key), vector.key_hex);
    assert_eq!(hex(&frame_nonce(1, vector.sequence)), vector.nonce_c2_n);
    assert_eq!(hex(&frame_nonce(2, vector.sequence)), vector.nonce_n2_c);
    assert_eq!(
        String::from_utf8(frame_aad(&vector.session_id, vector.sequence, "c2n")).unwrap(),
        vector.aad_c2_n
    );
    assert_eq!(
        encrypt_frame(
            &key,
            1,
            vector.sequence,
            &vector.session_id,
            "c2n",
            vector.plaintext.as_bytes()
        )
        .unwrap(),
        vector.ciphertext
    );
    assert_eq!(
        decrypt_frame(
            &key,
            1,
            vector.sequence,
            &vector.session_id,
            "c2n",
            &vector.ciphertext
        )
        .unwrap(),
        vector.plaintext.as_bytes()
    );
    assert_eq!(
        session_payload(
            &vector.challenge,
            &vector.device_id,
            &vector.client_id,
            &vector.client_public
        ),
        vector.session_payload
    );
    assert_eq!(
        ready_payload(
            &vector.challenge,
            &vector.device_id,
            &vector.client_id,
            &vector.client_public,
            &vector.node_static_public,
            &vector.node_ephemeral_public,
            &vector.session_id
        ),
        vector.ready_payload
    );
    assert_eq!(
        sign_ed25519_seed(&vector.node_identity_seed, vector.ready_payload.as_bytes()).unwrap(),
        vector.ready_signature
    );
    verify_ed25519(
        &vector.node_identity_public,
        vector.ready_payload.as_bytes(),
        &vector.ready_signature,
    )
    .unwrap();
}

fn base64url(value: &[u8]) -> String {
    use base64::Engine as _;
    base64::engine::general_purpose::URL_SAFE_NO_PAD.encode(value)
}
fn hex(value: &[u8]) -> String {
    value.iter().map(|b| format!("{b:02x}")).collect()
}
