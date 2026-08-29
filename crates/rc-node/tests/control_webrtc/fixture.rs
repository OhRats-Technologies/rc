use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use ed25519_dalek::SigningKey;
use rand::{RngCore, rngs::OsRng};
use rc_crypto::{
    derive_client_key, ready_payload, session_payload, sign_ed25519_seed, verify_ed25519,
    x25519_public,
};
use rc_node::{ControlManager, NodeState, ProcessEvent, ProcessManager, bootstrap_lock};
use rc_protocol::{
    AuthorityApiKey, AuthorityMember, AuthoritySnapshot, NodeToServer, ServerToNode,
};
use std::{path::PathBuf, sync::Arc, time::Duration};
use tokio::sync::mpsc;

pub(super) struct Harness {
    pub(super) state_dir: PathBuf,
    pub(super) node: NodeState,
    pub(super) control: ControlManager,
    pub(super) processes: Arc<ProcessManager>,
    pub(super) hosted: mpsc::UnboundedReceiver<NodeToServer>,
    signing_seed: String,
}

pub(super) struct Session {
    pub(super) id: String,
    pub(super) key: [u8; 32],
}

pub(super) fn setup() -> anyhow::Result<Harness> {
    let state_dir = temp_dir();
    let node = NodeState::generate("device".into());
    let signing = SigningKey::generate(&mut OsRng);
    let signing_seed = URL_SAFE_NO_PAD.encode(signing.to_bytes());
    let signing_public = URL_SAFE_NO_PAD.encode(signing.verifying_key().as_bytes());
    let snapshot = serde_json::to_string(&AuthoritySnapshot {
        v: 1,
        workspace_id: "workspace".into(),
        devices: Vec::new(),
        members: vec![AuthorityMember {
            user_id: "user".into(),
            role: "owner".into(),
            credentials: Vec::new(),
        }],
        api_keys: vec![AuthorityApiKey {
            id: "api".into(),
            user_id: "user".into(),
            public_key: signing_public,
            scopes: vec!["execute".into(), "manage-devices".into()],
            expires_at: 0,
        }],
        mcp_grants: Vec::new(),
    })?;
    bootstrap_lock(&state_dir, &snapshot, "https://rc.example.test")?;

    let (outbound, hosted) = mpsc::unbounded_channel();
    let event_outbound = outbound.clone();
    let runner = PathBuf::from(env!("CARGO_BIN_EXE_rc-process-runner"));
    let processes = Arc::new(ProcessManager::new(runner, move |event| {
        let message = match event {
            ProcessEvent::Started { id } => NodeToServer::ProcessStarted { id },
            ProcessEvent::Exit {
                id,
                exit_code,
                signal,
            } => NodeToServer::ProcessExit {
                id,
                exit_code,
                signal,
            },
            ProcessEvent::Stdout { .. } | ProcessEvent::Stderr { .. } => return,
        };
        let _ = event_outbound.send(message);
    }));
    let (process_policy, transport_policy) = crate::policies::pair();
    let control = ControlManager::new(
        node.clone(),
        state_dir.clone(),
        processes.clone(),
        outbound,
        "test",
        process_policy,
        transport_policy,
    );
    let secure_control = control.clone();
    processes.set_secure_sink(move |session_id, event| {
        secure_control.send_process_event(session_id, event)
    });

    Ok(Harness {
        state_dir,
        node,
        control,
        processes,
        hosted,
        signing_seed,
    })
}

pub(super) async fn open_control(harness: &mut Harness) -> anyhow::Result<Session> {
    harness
        .control
        .handle(
            "https://rc.example.test",
            ServerToNode::ControlChallenge {
                request_id: "challenge".into(),
            },
        )
        .await;
    let challenge = match recv_hosted(&mut harness.hosted).await? {
        NodeToServer::ControlChallenge {
            request_id,
            challenge,
        } => {
            assert_eq!(request_id, "challenge");
            challenge
        }
        other => anyhow::bail!("unexpected challenge response: {other:?}"),
    };

    let mut client_secret = [0_u8; 32];
    OsRng.fill_bytes(&mut client_secret);
    let client_private = URL_SAFE_NO_PAD.encode(client_secret);
    let client_public = x25519_public(&client_private)?;
    let signature = sign_ed25519_seed(
        &harness.signing_seed,
        session_payload(&challenge, "device", "api", &client_public).as_bytes(),
    )?;
    let open = ServerToNode::ControlOpen {
        request_id: "open".into(),
        challenge: challenge.clone(),
        user_id: "user".into(),
        client_id: "api".into(),
        grant: String::new(),
        credential_id: String::new(),
        assertion: String::new(),
        public_key: client_public.clone(),
        signature,
        ice_servers: Vec::new(),
    };
    harness
        .control
        .handle("https://rc.example.test", open.clone())
        .await;
    let (session_id, transport_public, ephemeral_public, ready_signature) =
        match recv_hosted(&mut harness.hosted).await? {
            NodeToServer::ControlReady {
                request_id,
                session_id,
                transport_public_key,
                ephemeral_public_key,
                signature,
                ..
            } => {
                assert_eq!(request_id, "open");
                (
                    session_id,
                    transport_public_key,
                    ephemeral_public_key,
                    signature,
                )
            }
            other => anyhow::bail!("unexpected open response: {other:?}"),
        };
    let ready = ready_payload(
        &challenge,
        "device",
        "api",
        &client_public,
        &transport_public,
        &ephemeral_public,
        &session_id,
        "host:2000:6000:0",
    );
    verify_ed25519(
        &harness.node.identity_public_key()?,
        ready.as_bytes(),
        &ready_signature,
    )?;
    let key = derive_client_key(
        &client_private,
        &transport_public,
        &ephemeral_public,
        &challenge,
        "device",
        "api",
    )?;

    harness
        .control
        .handle("https://rc.example.test", open)
        .await;
    assert!(matches!(
        recv_hosted(&mut harness.hosted).await?,
        NodeToServer::ControlError { .. }
    ));
    Ok(Session {
        id: session_id,
        key,
    })
}

pub(super) async fn recv_hosted(
    receiver: &mut mpsc::UnboundedReceiver<NodeToServer>,
) -> anyhow::Result<NodeToServer> {
    tokio::time::timeout(Duration::from_secs(5), receiver.recv())
        .await?
        .ok_or_else(|| anyhow::anyhow!("hosted channel closed"))
}

fn temp_dir() -> PathBuf {
    std::env::temp_dir().join(format!(
        "rc-control-{}-{}",
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_nanos()
    ))
}
