#[path = "control_authority/support.rs"]
mod support;

use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rand::{RngCore, rngs::OsRng};
use rc_crypto::sign_ed25519_seed;
use rc_node::{
    ControlManager, LockError, NodeState, ProcessManager, bootstrap_lock, load_lock, snapshot_hash,
    sync_lock, verify_control_proof,
};
use rc_protocol::{AuthorityApiKey, NodeToServer, ServerToNode};
use std::{path::PathBuf, sync::Arc};
use support::{fixture, recv_hosted, temp_dir};
use tokio::sync::mpsc;

#[test]
fn passkey_control_grant_matches_existing_es256_semantics() -> anyhow::Result<()> {
    let owner_fixture = fixture("owner")?;
    let authority = verify_control_proof(
        &owner_fixture.snapshot,
        &owner_fixture.proof,
        "https://rc.ohrats.party",
        "rc.ohrats.party",
    )?;
    assert_eq!(authority.role, "owner");
    assert_eq!(authority.grant.client_id, "client");

    assert!(matches!(
        verify_control_proof(
            &owner_fixture.snapshot,
            &owner_fixture.proof,
            "https://evil.invalid",
            "rc.ohrats.party",
        ),
        Err(LockError::Passkey)
    ));
    Ok(())
}

#[test]
fn owner_signed_lock_sync_rejects_replay_and_operator_transition() -> anyhow::Result<()> {
    let owner_fixture = fixture("owner")?;
    let dir = temp_dir("owner");
    let initial = serde_json::to_string(&owner_fixture.snapshot)?;
    bootstrap_lock(&dir, &initial, "https://rc.ohrats.party")?;

    let mut next = owner_fixture.snapshot.clone();
    next.api_keys.push(AuthorityApiKey {
        id: "temporary".into(),
        user_id: "user".into(),
        public_key: URL_SAFE_NO_PAD.encode(owner_fixture.client_signing.verifying_key().as_bytes()),
        scopes: vec!["read".into()],
        expires_at: 0,
    });
    let next_json = serde_json::to_string(&next)?;
    let previous_hash = snapshot_hash(&initial);
    let payload = format!(
        "rc-authority-v3\n0\n{}\n{}",
        previous_hash,
        snapshot_hash(&next_json)
    );
    let seed = URL_SAFE_NO_PAD.encode(owner_fixture.client_signing.to_bytes());
    let signature = sign_ed25519_seed(&seed, payload.as_bytes())?;
    sync_lock(
        &dir,
        &next_json,
        &previous_hash,
        0,
        &owner_fixture.proof,
        &signature,
    )?;
    let locked = load_lock(&dir)?;
    assert_eq!(locked.generation, 1);
    assert_eq!(locked.snapshot, next_json);
    assert!(matches!(
        sync_lock(
            &dir,
            &next_json,
            &previous_hash,
            0,
            &owner_fixture.proof,
            &signature,
        ),
        Err(LockError::StaleTransition)
    ));
    let _ = std::fs::remove_dir_all(&dir);

    let operator = fixture("operator")?;
    let dir = temp_dir("operator");
    let initial = serde_json::to_string(&operator.snapshot)?;
    bootstrap_lock(&dir, &initial, "https://rc.ohrats.party")?;
    let mut next = operator.snapshot.clone();
    next.members[0].role = "owner".into();
    let next_json = serde_json::to_string(&next)?;
    let previous_hash = snapshot_hash(&initial);
    let payload = format!(
        "rc-authority-v3\n0\n{}\n{}",
        previous_hash,
        snapshot_hash(&next_json)
    );
    let seed = URL_SAFE_NO_PAD.encode(operator.client_signing.to_bytes());
    let signature = sign_ed25519_seed(&seed, payload.as_bytes())?;
    assert!(matches!(
        sync_lock(
            &dir,
            &next_json,
            &previous_hash,
            0,
            &operator.proof,
            &signature,
        ),
        Err(LockError::OwnerRequired)
    ));
    let _ = std::fs::remove_dir_all(dir);
    Ok(())
}

#[tokio::test]
async fn live_passkey_session_is_revoked_by_owner_lock_transition() -> anyhow::Result<()> {
    let fixture = fixture("owner")?;
    let dir = temp_dir("live");
    let initial = serde_json::to_string(&fixture.snapshot)?;
    bootstrap_lock(&dir, &initial, "https://rc.ohrats.party")?;
    let node = NodeState::generate("device".into());
    let processes = Arc::new(ProcessManager::new(
        PathBuf::from(env!("CARGO_BIN_EXE_rc-process-runner")),
        |_| {},
    ));
    let (outbound, mut hosted) = mpsc::unbounded_channel();
    let control = ControlManager::new(
        node.clone(),
        dir.clone(),
        processes.clone(),
        outbound,
        "test",
    );

    control
        .handle(
            "https://rc.ohrats.party",
            ServerToNode::ControlChallenge {
                request_id: "challenge".into(),
            },
        )
        .await;
    let challenge = match recv_hosted(&mut hosted).await? {
        NodeToServer::ControlChallenge { challenge, .. } => challenge,
        other => anyhow::bail!("unexpected challenge response: {other:?}"),
    };
    let mut transport_secret = [0_u8; 32];
    OsRng.fill_bytes(&mut transport_secret);
    let transport_private = URL_SAFE_NO_PAD.encode(transport_secret);
    let transport_public = rc_crypto::x25519_public(&transport_private)?;
    let signing_seed = URL_SAFE_NO_PAD.encode(fixture.client_signing.to_bytes());
    let session_signature = sign_ed25519_seed(
        &signing_seed,
        rc_crypto::session_payload(&challenge, "device", "client", &transport_public).as_bytes(),
    )?;
    control
        .handle(
            "https://rc.ohrats.party",
            ServerToNode::ControlOpen {
                request_id: "open".into(),
                challenge,
                user_id: "user".into(),
                client_id: "client".into(),
                grant: fixture.proof.grant.clone(),
                credential_id: fixture.proof.credential_id.clone(),
                assertion: fixture.proof.assertion.clone(),
                public_key: transport_public,
                signature: session_signature,
            },
        )
        .await;
    let session_id = match recv_hosted(&mut hosted).await? {
        NodeToServer::ControlReady { session_id, .. } => session_id,
        other => anyhow::bail!("passkey control open failed: {other:?}"),
    };
    assert!(control.has_session(&session_id));

    let mut next = fixture.snapshot.clone();
    next.api_keys.push(AuthorityApiKey {
        id: "new-key".into(),
        user_id: "user".into(),
        public_key: URL_SAFE_NO_PAD.encode(fixture.client_signing.verifying_key().as_bytes()),
        scopes: vec!["read".into()],
        expires_at: 0,
    });
    let next_json = serde_json::to_string(&next)?;
    let previous_hash = snapshot_hash(&initial);
    let transition_payload = format!(
        "rc-authority-v3\n0\n{}\n{}",
        previous_hash,
        snapshot_hash(&next_json)
    );
    let transition_signature = sign_ed25519_seed(&signing_seed, transition_payload.as_bytes())?;
    control
        .handle(
            "https://rc.ohrats.party",
            ServerToNode::LockSync {
                snapshot: next_json,
                previous_hash,
                previous_generation: 0,
                grant: fixture.proof.grant,
                credential_id: fixture.proof.credential_id,
                assertion: fixture.proof.assertion,
                signature: transition_signature,
            },
        )
        .await;
    assert!(matches!(
        recv_hosted(&mut hosted).await?,
        NodeToServer::ControlClosed { session_id: ref id } if id == &session_id
    ));
    assert!(matches!(
        recv_hosted(&mut hosted).await?,
        NodeToServer::LockState { generation: 1, .. }
    ));
    assert!(!control.has_session(&session_id));
    assert_eq!(load_lock(&dir)?.generation, 1);

    control.shutdown().await;
    processes.shutdown();
    let _ = std::fs::remove_dir_all(dir);
    Ok(())
}
