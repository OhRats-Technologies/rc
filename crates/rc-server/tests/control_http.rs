use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use ed25519_dalek::SigningKey;
use rc_crypto::sign_api_seed;
use rc_node::{NodeState, ServerTransport};
use rc_protocol::{NodeToServer, ServerToNode};
use rc_server::{AppState, Config, ControlHub, NewDevice, TurnProvider, app, now_ms};
use serde_json::Value;
use std::{net::SocketAddr, path::PathBuf, time::Duration};
use tokio::time::timeout;
use uuid::Uuid;

#[tokio::test]
async fn signed_control_http_routes_drive_live_node_signaling() -> anyhow::Result<()> {
    let root = std::env::temp_dir().join(format!("rc-control-http-{}", Uuid::new_v4()));
    std::fs::create_dir_all(&root)?;
    let _cleanup = Cleanup(root.clone());
    let db_path = root.join("rc.sqlite3");
    let mut state = AppState::new(Config {
        listen: SocketAddr::from(([127, 0, 0, 1], 0)),
        data_dir: root,
        db_path: db_path.clone(),
        public_url: "http://localhost".into(),
        static_dir: std::path::PathBuf::from("dist/assets"),
        trust_proxy: false,
        setup_token: None,
        public_signup: false,
        turnstile_site_key: None,
        turnstile_secret_key: None,
        turn_token_id: None,
        turn_api_token: None,
        ssh_daemon_port: 2222,
        ssh_internal_port: 3001,
        mcp_access_ttl_minutes: 15,
        execution_history: rc_server::ExecutionHistory::None,
        execution_history_ttl_hours: 168,
    })?;
    state.turn = TurnProvider::fixed(Vec::new());
    state.control = ControlHub::new(state.nodes.clone(), state.turn.clone());

    let user_id = "user";
    let workspace_id = "workspace";
    let client_id = "api-client";
    let signing = SigningKey::from_bytes(&[13_u8; 32]);
    let signing_seed = URL_SAFE_NO_PAD.encode(signing.to_bytes());
    let db = rusqlite::Connection::open(&db_path)?;
    db.execute("PRAGMA foreign_keys=ON", [])?;
    db.execute(
        "INSERT INTO users(id,name,created_at) VALUES(?,?,?)",
        rusqlite::params![user_id, "Test User", now_ms()],
    )?;
    db.execute(
        "INSERT INTO workspaces(id,name,created_by,created_at) VALUES(?,?,?,?)",
        rusqlite::params![workspace_id, "Test Workspace", user_id, now_ms()],
    )?;
    db.execute(
        "INSERT INTO workspace_members(workspace_id,user_id,role,joined_at) VALUES(?,?,?,?)",
        rusqlite::params![workspace_id, user_id, "owner", now_ms()],
    )?;
    db.execute(
        "INSERT INTO clients(id,user_id,kind,name,public_key,scopes,created_at) VALUES(?,?,?,?,?,?,?)",
        rusqlite::params![
            client_id,
            user_id,
            "api",
            "Test API",
            URL_SAFE_NO_PAD.encode(signing.verifying_key().as_bytes()),
            r#"["execute"]"#,
            now_ms()
        ],
    )?;

    let device_id = Uuid::new_v4().to_string();
    let node = NodeState::generate(device_id.clone());
    state.db.insert_device(&NewDevice {
        id: device_id.clone(),
        workspace_id: workspace_id.into(),
        name: "test-node".into(),
        hostname: "test-host".into(),
        platform: "test".into(),
        arch: "test".into(),
        identity_public_key: node.identity_public_key()?,
        transport_public_key: node.transport_public_key()?,
        version: "test".into(),
        capabilities: vec!["webrtc".into()],
    })?;

    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await?;
    let address = listener.local_addr()?;
    let server = tokio::spawn(axum::serve(listener, app(state.clone())).into_future());
    let base = format!("http://{address}");
    let mut node_transport = timeout(
        Duration::from_secs(10),
        ServerTransport::connect(&base, &node),
    )
    .await??;
    wait_online(&state, &device_id).await?;

    let challenge_body = serde_json::to_vec(&serde_json::json!({ "deviceId": device_id }))?;
    let challenge_request = signed_request(
        reqwest::Client::new(),
        reqwest::Method::POST,
        &base,
        "/api/v1/control/challenge",
        &challenge_body,
        client_id,
        &signing_seed,
        1,
    )?;
    let challenge_task = tokio::spawn(async move { challenge_request.send().await });
    let request_id = match timeout(Duration::from_secs(3), node_transport.recv()).await? {
        Some(ServerToNode::ControlChallenge { request_id }) => request_id,
        other => anyhow::bail!("unexpected challenge request: {other:?}"),
    };
    node_transport
        .send(&NodeToServer::ControlChallenge {
            request_id,
            challenge: "http-challenge".into(),
        })
        .await?;
    let challenge_response = challenge_task.await??;
    assert_eq!(challenge_response.status(), reqwest::StatusCode::OK);
    let challenge_json: Value = challenge_response.json().await?;
    assert_eq!(challenge_json["challenge"], "http-challenge");

    let open_body = serde_json::to_vec(&serde_json::json!({
        "deviceId": device_id,
        "challenge": "http-challenge",
        "clientId": client_id,
        "publicKey": "client-transport",
        "signature": "client-signature"
    }))?;
    let open_request = signed_request(
        reqwest::Client::new(),
        reqwest::Method::POST,
        &base,
        "/api/v1/control/open",
        &open_body,
        client_id,
        &signing_seed,
        2,
    )?;
    let open_task = tokio::spawn(async move { open_request.send().await });
    let request_id = match timeout(Duration::from_secs(3), node_transport.recv()).await? {
        Some(ServerToNode::ControlOpen {
            request_id,
            user_id: message_user,
            client_id: message_client,
            grant,
            ..
        }) => {
            assert_eq!(message_user, user_id);
            assert_eq!(message_client, client_id);
            assert!(grant.is_empty());
            request_id
        }
        other => anyhow::bail!("unexpected open request: {other:?}"),
    };
    node_transport
        .send(&NodeToServer::ControlReady {
            request_id,
            session_id: "http-session".into(),
            transport_public_key: "node-transport".into(),
            ephemeral_public_key: "node-ephemeral".into(),
            signature: "node-signature".into(),
        })
        .await?;
    let open_response = open_task.await??;
    assert_eq!(open_response.status(), reqwest::StatusCode::OK);
    let open_json: Value = open_response.json().await?;
    assert_eq!(open_json["sessionId"], "http-session");
    assert_eq!(open_json["iceServers"], serde_json::json!([]));

    let webrtc_path = "/api/v1/control/http-session/webrtc";
    let webrtc_body = serde_json::to_vec(&serde_json::json!({
        "deviceId": device_id,
        "sdp": "browser-offer"
    }))?;
    let webrtc_request = signed_request(
        reqwest::Client::new(),
        reqwest::Method::POST,
        &base,
        webrtc_path,
        &webrtc_body,
        client_id,
        &signing_seed,
        3,
    )?;
    let webrtc_task = tokio::spawn(async move { webrtc_request.send().await });
    let request_id = match timeout(Duration::from_secs(3), node_transport.recv()).await? {
        Some(ServerToNode::ControlWebrtcOffer {
            request_id,
            session_id,
            sdp,
            ..
        }) => {
            assert_eq!(session_id, "http-session");
            assert_eq!(sdp, "browser-offer");
            request_id
        }
        other => anyhow::bail!("unexpected WebRTC request: {other:?}"),
    };
    node_transport
        .send(&NodeToServer::ControlWebrtcAnswer {
            request_id,
            session_id: "http-session".into(),
            sdp: "node-answer".into(),
        })
        .await?;
    let webrtc_response = webrtc_task.await??;
    assert_eq!(webrtc_response.status(), reqwest::StatusCode::OK);
    let webrtc_json: Value = webrtc_response.json().await?;
    assert_eq!(webrtc_json["sdp"], "node-answer");

    let close_path = "/api/v1/control/http-session";
    let close_request = signed_request(
        reqwest::Client::new(),
        reqwest::Method::DELETE,
        &base,
        close_path,
        &[],
        client_id,
        &signing_seed,
        4,
    )?;
    let close_response = close_request.send().await?;
    assert_eq!(close_response.status(), reqwest::StatusCode::OK);
    assert!(matches!(
        timeout(Duration::from_secs(3), node_transport.recv()).await?,
        Some(ServerToNode::ControlClose { session_id }) if session_id == "http-session"
    ));
    assert!(!state.control.has_session("http-session"));

    node_transport.close().await;
    server.abort();
    Ok(())
}

#[allow(clippy::too_many_arguments)]
fn signed_request(
    client: reqwest::Client,
    method: reqwest::Method,
    base: &str,
    path: &str,
    body: &[u8],
    client_id: &str,
    signing_seed: &str,
    nonce_id: u8,
) -> anyhow::Result<reqwest::RequestBuilder> {
    let timestamp = (now_ms() / 1000).to_string();
    let nonce = format!("control-http-nonce-{nonce_id:02}-abcdefghijkl");
    let signature = sign_api_seed(
        signing_seed,
        client_id,
        &timestamp,
        &nonce,
        method.as_str(),
        path,
        body,
    )?;
    let mut request = client
        .request(method, format!("{base}{path}"))
        .header("x-rc-key-id", client_id)
        .header("x-rc-timestamp", timestamp)
        .header("x-rc-nonce", nonce)
        .header("x-rc-signature", signature);
    if !body.is_empty() {
        request = request
            .header("content-type", "application/json")
            .body(body.to_vec());
    }
    Ok(request)
}

async fn wait_online(state: &AppState, device_id: &str) -> anyhow::Result<()> {
    timeout(Duration::from_secs(3), async {
        loop {
            if state.nodes.online(device_id).await {
                return;
            }
            tokio::time::sleep(Duration::from_millis(20)).await;
        }
    })
    .await
    .map_err(|_| anyhow::anyhow!("Node did not become online"))?;
    Ok(())
}

struct Cleanup(PathBuf);

impl Drop for Cleanup {
    fn drop(&mut self) {
        let _ = std::fs::remove_dir_all(&self.0);
    }
}
