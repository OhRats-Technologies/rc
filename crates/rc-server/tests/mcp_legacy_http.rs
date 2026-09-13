use axum::{
    body::{Body, to_bytes},
    http::{Request, StatusCode, header},
};
use rc_protocol::McpGrantPayload;
use rc_server::{AppState, Config, app, hash, now_ms};
use std::{net::SocketAddr, path::PathBuf};
use tower::ServiceExt;
use uuid::Uuid;

const LEGACY_VERSION: &str = "2025-11-25";

#[tokio::test]
async fn legacy_streamable_http_lists_and_calls_tools() -> anyhow::Result<()> {
    let root = temp_root()?;
    let db_path = root.join("rc.sqlite3");
    let state = AppState::new(test_config(&root, &db_path))?;
    let access = "legacy-access-token";
    seed(&db_path, access)?;
    let application = app(state);

    let initialize = application
        .clone()
        .oneshot(
            Request::post("/mcp")
                .header(header::CONTENT_TYPE, "application/json")
                .body(json_body(serde_json::json!({
                    "jsonrpc":"2.0","id":1,"method":"initialize","params":{
                        "protocolVersion":LEGACY_VERSION,
                        "capabilities":{},
                        "clientInfo":{"name":"legacy-test","version":"1.0"}
                    }
                }))?)?,
        )
        .await?;
    assert_eq!(initialize.status(), StatusCode::OK);
    let initialize = response_json(initialize).await?;
    assert_eq!(initialize["result"]["protocolVersion"], LEGACY_VERSION);
    assert!(initialize["result"].get("resultType").is_none());

    let initialized = application
        .clone()
        .oneshot(
            Request::post("/mcp")
                .header(header::CONTENT_TYPE, "application/json")
                .header("mcp-protocol-version", LEGACY_VERSION)
                .body(json_body(serde_json::json!({
                    "jsonrpc":"2.0","method":"notifications/initialized","params":{}
                }))?)?,
        )
        .await?;
    assert_eq!(initialized.status(), StatusCode::ACCEPTED);

    let tools = application
        .clone()
        .oneshot(
            Request::post("/mcp")
                .header(header::CONTENT_TYPE, "application/json")
                .header("mcp-protocol-version", LEGACY_VERSION)
                .header(header::AUTHORIZATION, format!("Bearer {access}"))
                .body(json_body(serde_json::json!({
                    "jsonrpc":"2.0","id":2,"method":"tools/list","params":{}
                }))?)?,
        )
        .await?;
    assert_eq!(tools.status(), StatusCode::OK);
    let tools = response_json(tools).await?;
    assert!(tools["result"]["tools"].is_array());
    assert!(tools["result"].get("resultType").is_none());
    assert!(tools["result"].get("ttlMs").is_none());

    let machines = application
        .oneshot(
            Request::post("/mcp")
                .header(header::CONTENT_TYPE, "application/json")
                .header("mcp-protocol-version", LEGACY_VERSION)
                .header(header::AUTHORIZATION, format!("Bearer {access}"))
                .body(json_body(serde_json::json!({
                    "jsonrpc":"2.0","id":3,"method":"tools/call","params":{
                        "name":"machines_list","arguments":{}
                    }
                }))?)?,
        )
        .await?;
    assert_eq!(machines.status(), StatusCode::OK);
    let machines = response_json(machines).await?;
    assert!(machines["result"]["structuredContent"]["machines"].is_array());
    assert!(machines["result"].get("resultType").is_none());
    Ok(())
}

fn seed(db_path: &std::path::Path, access: &str) -> anyhow::Result<()> {
    let db = rusqlite::Connection::open(db_path)?;
    db.execute("PRAGMA foreign_keys=ON", [])?;
    let now = now_ms();
    let user = "legacy-user";
    let client = "legacy-client";
    let grant_id = "legacy-grant";
    db.execute(
        "INSERT INTO users(id,name,created_at) VALUES(?,?,?)",
        rusqlite::params![user, "Legacy User", now],
    )?;
    db.execute(
        "INSERT INTO mcp_clients(id,name,redirect_uris,created_at) VALUES(?,?,?,?)",
        rusqlite::params![client, "Legacy Client", "[]", now],
    )?;
    let grant = serde_json::to_string(&McpGrantPayload {
        v: 1,
        id: grant_id.into(),
        user_id: user.into(),
        client_id: client.into(),
        client_name: "Legacy Client".into(),
        device_ids: Vec::new(),
        scopes: vec!["mcp:observe".into()],
        issued_at: now,
        expires_at: now + 60 * 60_000,
    })?;
    db.execute(
        "INSERT INTO mcp_grants(id,user_id,client_id,name,grant,grant_signature,client_control_id,credential_id,control_grant,control_assertion,created_at,expires_at) VALUES(?,?,?,?,?,'signature','control','credential','control-grant','assertion',?,?)",
        rusqlite::params![grant_id, user, client, "Legacy Client", grant, now, now + 60 * 60_000],
    )?;
    db.execute(
        "INSERT INTO oauth_tokens(token_hash,grant_id,kind,expires_at) VALUES(?,?,'access',?)",
        rusqlite::params![hash(access), grant_id, now + 15 * 60_000],
    )?;
    Ok(())
}

fn json_body(value: serde_json::Value) -> anyhow::Result<Body> {
    Ok(Body::from(serde_json::to_vec(&value)?))
}

async fn response_json(response: axum::response::Response) -> anyhow::Result<serde_json::Value> {
    let bytes = to_bytes(response.into_body(), 2 * 1024 * 1024).await?;
    Ok(serde_json::from_slice(&bytes)?)
}

fn test_config(root: &std::path::Path, db_path: &std::path::Path) -> Config {
    Config {
        listen: SocketAddr::from(([127, 0, 0, 1], 0)),
        data_dir: root.to_path_buf(),
        db_path: db_path.to_path_buf(),
        public_url: "http://localhost".into(),
        static_dir: root.to_path_buf(),
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
    }
}

fn temp_root() -> anyhow::Result<PathBuf> {
    let root = std::env::temp_dir().join(format!("rc-mcp-legacy-{}", Uuid::new_v4()));
    std::fs::create_dir_all(&root)?;
    Ok(root)
}
