use rc_node::{NodeState, ServerTransport};
use rc_protocol::{McpGrantPayload, ServerToNode};
use rc_server::{
    AppState, Config, MCP_PROTOCOL_VERSION, NewDevice, TurnProvider, app, hash, now_ms,
};
use std::{net::SocketAddr, path::PathBuf, time::Duration};
use tokio::time::timeout;
use uuid::Uuid;

pub struct Harness {
    root: PathBuf,
    server: tokio::task::JoinHandle<()>,
    pub node: NodeState,
    pub device_id: String,
    pub access_token: String,
    pub base: String,
}

impl Harness {
    pub async fn start() -> anyhow::Result<Self> {
        let root = std::env::temp_dir().join(format!("rc-mcp-process-{}", Uuid::new_v4()));
        std::fs::create_dir_all(&root)?;
        let db_path = root.join("rc.sqlite3");
        let config = Config {
            listen: SocketAddr::from(([127, 0, 0, 1], 0)),
            data_dir: root.clone(),
            db_path: db_path.clone(),
            public_url: "http://localhost".into(),
            static_dir: PathBuf::from("dist/assets"),
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
        };
        let mut state = AppState::new(config)?;
        state.turn = TurnProvider::fixed(Vec::new());
        let user_id = Uuid::new_v4().to_string();
        let workspace_id = Uuid::new_v4().to_string();
        let device_id = Uuid::new_v4().to_string();
        let node = NodeState::generate(device_id.clone());
        seed(&state, &db_path, &user_id, &workspace_id, &device_id, &node)?;
        let access_token = "mcp_access_process_test".to_owned();
        seed_grant(&db_path, &access_token, &user_id, &device_id)?;
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await?;
        let address = listener.local_addr()?;
        let server = tokio::spawn(async move {
            let _ = axum::serve(listener, app(state)).await;
        });
        Ok(Self {
            root,
            server,
            node,
            device_id,
            access_token,
            base: format!("http://{address}"),
        })
    }
}

impl Drop for Harness {
    fn drop(&mut self) {
        self.server.abort();
        let _ = std::fs::remove_dir_all(&self.root);
    }
}

pub async fn recv(node: &mut ServerTransport) -> anyhow::Result<ServerToNode> {
    timeout(Duration::from_secs(3), node.recv())
        .await?
        .ok_or_else(|| anyhow::anyhow!("Node transport closed"))
}

pub async fn rpc_call(
    client: &reqwest::Client,
    harness: &Harness,
    name: &str,
    arguments: serde_json::Value,
    id: u64,
) -> anyhow::Result<serde_json::Value> {
    let response = client
        .post(format!("{}/mcp", harness.base))
        .bearer_auth(&harness.access_token)
        .header("mcp-protocol-version", MCP_PROTOCOL_VERSION)
        .header("mcp-method", "tools/call")
        .header("mcp-name", name)
        .json(&serde_json::json!({
            "jsonrpc": "2.0",
            "id": id,
            "method": "tools/call",
            "params": {"name": name, "arguments": arguments},
        }))
        .send()
        .await?;
    anyhow::ensure!(response.status().is_success(), "MCP call failed");
    let value: serde_json::Value = response.json().await?;
    anyhow::ensure!(value.get("error").is_none(), "MCP RPC error: {value}");
    anyhow::ensure!(
        value["result"]["isError"] != true,
        "MCP tool error: {value}"
    );
    Ok(value)
}

fn seed(
    state: &AppState,
    db_path: &std::path::Path,
    user: &str,
    workspace: &str,
    device: &str,
    node: &NodeState,
) -> anyhow::Result<()> {
    let db = rusqlite::Connection::open(db_path)?;
    db.execute("PRAGMA foreign_keys=ON", [])?;
    db.execute(
        "INSERT INTO users(id,name,created_at) VALUES(?,?,?)",
        rusqlite::params![user, "MCP User", now_ms()],
    )?;
    db.execute(
        "INSERT INTO workspaces(id,name,created_by,created_at) VALUES(?,?,?,?)",
        rusqlite::params![workspace, "MCP Workspace", user, now_ms()],
    )?;
    db.execute(
        "INSERT INTO workspace_members(workspace_id,user_id,role,joined_at) VALUES(?,?,'owner',?)",
        rusqlite::params![workspace, user, now_ms()],
    )?;
    drop(db);
    state.db.insert_device(&NewDevice {
        id: device.into(),
        workspace_id: workspace.into(),
        name: "MCP Node".into(),
        hostname: "mcp-node".into(),
        platform: "darwin".into(),
        arch: "arm64".into(),
        identity_public_key: node.identity_public_key()?,
        transport_public_key: node.transport_public_key()?,
        version: "0.18.0-test".into(),
        capabilities: vec!["process".into(), "webrtc".into()],
    })?;
    Ok(())
}

fn seed_grant(
    db_path: &std::path::Path,
    token: &str,
    user: &str,
    device: &str,
) -> anyhow::Result<()> {
    let grant_id = "mcp-process-grant";
    let client_id = "mcp-process-client";
    let grant = serde_json::to_string(&McpGrantPayload {
        v: 1,
        id: grant_id.into(),
        user_id: user.into(),
        client_id: client_id.into(),
        client_name: "MCP Process Test".into(),
        device_ids: vec![device.into()],
        scopes: vec!["mcp:observe".into(), "mcp:terminal".into()],
        issued_at: now_ms(),
        expires_at: now_ms() + 60_000,
    })?;
    let db = rusqlite::Connection::open(db_path)?;
    db.execute("PRAGMA foreign_keys=ON", [])?;
    db.execute(
        "INSERT INTO mcp_clients(id,name,redirect_uris,created_at) VALUES(?,?,'[]',?)",
        rusqlite::params![client_id, "MCP Process Test", now_ms()],
    )?;
    db.execute(
        "INSERT INTO mcp_grants(id,user_id,client_id,name,grant,grant_signature,client_control_id,credential_id,control_grant,control_assertion,created_at,expires_at) VALUES(?,?,?,?,?,'signature','control','credential','control-grant','assertion',?,?)",
        rusqlite::params![grant_id,user,client_id,"MCP Process Test",grant,now_ms(),now_ms()+60_000],
    )?;
    db.execute(
        "INSERT INTO oauth_tokens(token_hash,grant_id,kind,expires_at) VALUES(?,?,'access',?)",
        rusqlite::params![hash(token), grant_id, now_ms() + 60_000],
    )?;
    Ok(())
}
