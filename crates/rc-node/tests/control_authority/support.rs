use base64::{
    Engine as _,
    engine::general_purpose::{STANDARD, URL_SAFE_NO_PAD},
};
use ed25519_dalek::SigningKey as Ed25519SigningKey;
use p256::ecdsa::{Signature as P256Signature, SigningKey as P256SigningKey, signature::Signer};
use rand::{RngCore, rngs::OsRng};
use rc_protocol::{
    AuthorityCredential, AuthorityMember, AuthoritySnapshot, ControlGrant, ControlProof,
    NodeToServer,
};
use sha2::{Digest, Sha256};
use std::{
    path::PathBuf,
    time::{SystemTime, UNIX_EPOCH},
};
use tokio::sync::mpsc;
use webauthn_rs_core::proto::{COSEAlgorithm, COSEEC2Key, COSEKey, COSEKeyType, ECDSACurve};

pub(super) struct Fixture {
    pub(super) snapshot: AuthoritySnapshot,
    pub(super) proof: ControlProof,
    pub(super) client_signing: Ed25519SigningKey,
}

pub(super) fn fixture(role: &str) -> anyhow::Result<Fixture> {
    let passkey = P256SigningKey::random(&mut OsRng);
    let point = passkey.verifying_key().to_encoded_point(false);
    let x = point
        .x()
        .ok_or_else(|| anyhow::anyhow!("missing x coordinate"))?;
    let y = point
        .y()
        .ok_or_else(|| anyhow::anyhow!("missing y coordinate"))?;
    let cose = COSEKey {
        type_: COSEAlgorithm::ES256,
        key: COSEKeyType::EC_EC2(COSEEC2Key {
            curve: ECDSACurve::SECP256R1,
            x: x.to_vec().into(),
            y: y.to_vec().into(),
        }),
    };

    let mut credential_bytes = [0_u8; 32];
    OsRng.fill_bytes(&mut credential_bytes);
    let credential_id = URL_SAFE_NO_PAD.encode(credential_bytes);
    let client_signing = Ed25519SigningKey::generate(&mut OsRng);
    let now = now_ms();
    let grant = serde_json::to_string(&ControlGrant {
        v: 1,
        client_id: "client".into(),
        user_id: "user".into(),
        signing_public_key: URL_SAFE_NO_PAD.encode(client_signing.verifying_key().as_bytes()),
        issued_at: now,
        expires_at: now + 30 * 24 * 60 * 60 * 1000,
    })?;
    let assertion = assertion_for_grant(
        &passkey,
        &credential_id,
        &grant,
        "https://rc.ohrats.party",
        "rc.ohrats.party",
    )?;
    Ok(Fixture {
        snapshot: AuthoritySnapshot {
            v: 1,
            workspace_id: "workspace".into(),
            members: vec![AuthorityMember {
                user_id: "user".into(),
                role: role.into(),
                credentials: vec![AuthorityCredential {
                    id: credential_id.clone(),
                    public_key: STANDARD.encode(serde_json::to_vec(&cose)?),
                }],
            }],
            api_keys: Vec::new(),
            mcp_grants: Vec::new(),
        },
        proof: ControlProof {
            grant,
            credential_id,
            assertion,
        },
        client_signing,
    })
}

fn assertion_for_grant(
    signing: &P256SigningKey,
    credential_id: &str,
    grant: &str,
    origin: &str,
    rp_id: &str,
) -> anyhow::Result<String> {
    let client_json = serde_json::to_vec(&serde_json::json!({
        "type": "webauthn.get",
        "challenge": rc_crypto::control_grant_challenge(grant),
        "origin": origin,
        "crossOrigin": false,
    }))?;
    let mut auth_data = vec![0_u8; 37];
    auth_data[..32].copy_from_slice(&Sha256::digest(rp_id.as_bytes()));
    auth_data[32] = 0x05;
    let client_hash = Sha256::digest(&client_json);
    let mut signed = auth_data.clone();
    signed.extend_from_slice(&client_hash);
    let signature: P256Signature = signing.sign(&signed);
    Ok(serde_json::to_string(&serde_json::json!({
        "id": credential_id,
        "rawId": credential_id,
        "type": "public-key",
        "response": {
            "clientDataJSON": URL_SAFE_NO_PAD.encode(client_json),
            "authenticatorData": URL_SAFE_NO_PAD.encode(auth_data),
            "signature": URL_SAFE_NO_PAD.encode(signature.to_der().as_bytes()),
        }
    }))?)
}

fn now_ms() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
}

pub(super) fn temp_dir(label: &str) -> PathBuf {
    std::env::temp_dir().join(format!(
        "rc-control-authority-{label}-{}-{}",
        std::process::id(),
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_nanos()
    ))
}

pub(super) async fn recv_hosted(
    receiver: &mut mpsc::UnboundedReceiver<NodeToServer>,
) -> anyhow::Result<NodeToServer> {
    tokio::time::timeout(std::time::Duration::from_secs(5), receiver.recv())
        .await?
        .ok_or_else(|| anyhow::anyhow!("hosted channel closed"))
}
