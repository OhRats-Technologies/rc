use axum::{
    body::{Body, to_bytes},
    http::{Request, StatusCode, header},
};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_protocol::McpGrantPayload;
use rc_server::{AppState, Config, MCP_PROTOCOL_VERSION, app, hash, now_ms};
use sha2::{Digest, Sha256};
use std::{net::SocketAddr, path::PathBuf};
use tower::ServiceExt;
use uuid::Uuid;

#[tokio::test]
async fn oauth_codes_and_refresh_tokens_are_single_use_and_resource_bound() -> anyhow::Result<()> {
    let root = temp_root()?;
    let db_path = root.join("rc.sqlite3");
    let state = AppState::new(test_config(&root, &db_path))?;
    let resource = "http://localhost/mcp";
    let client = "mcp-client-test";
    let redirect = "http://localhost/callback";
    let verifier = "a".repeat(43);
    let code = "mcp_code_test";
    seed(&db_path, client, redirect, resource, &verifier, code)?;
    let application = app(state);

    let first = token(
        &application,
        &[
            ("grant_type", "authorization_code"),
            ("client_id", client),
            ("code", code),
            ("redirect_uri", redirect),
            ("code_verifier", &verifier),
            ("resource", resource),
        ],
    )
    .await?;
    assert_eq!(first.status, StatusCode::OK);
    let access = first.body["access_token"]
        .as_str()
        .ok_or_else(|| anyhow::anyhow!("missing access token"))?;
    let refresh = first.body["refresh_token"]
        .as_str()
        .ok_or_else(|| anyhow::anyhow!("missing refresh token"))?;

    let replay = token(
        &application,
        &[
            ("grant_type", "authorization_code"),
            ("client_id", client),
            ("code", code),
            ("redirect_uri", redirect),
            ("code_verifier", &verifier),
            ("resource", resource),
        ],
    )
    .await?;
    assert_eq!(replay.status, StatusCode::BAD_REQUEST);

    let tools = application
        .clone()
        .oneshot(
            Request::post("/mcp")
                .header(header::CONTENT_TYPE, "application/json")
                .header("mcp-protocol-version", MCP_PROTOCOL_VERSION)
                .header("mcp-method", "tools/list")
                .header(header::AUTHORIZATION, format!("Bearer {access}"))
                .body(Body::from(serde_json::to_vec(&serde_json::json!({
                    "jsonrpc":"2.0","id":1,"method":"tools/list","params":{}
                }))?))?,
        )
        .await?;
    assert_eq!(tools.status(), StatusCode::OK);
    let tools = response_json(tools).await?;
    let listed = tools["result"]["tools"]
        .as_array()
        .ok_or_else(|| anyhow::anyhow!("missing MCP tools"))?;
    let names: Vec<_> = listed
        .iter()
        .filter_map(|tool| tool["name"].as_str())
        .collect();
    assert_eq!(
        names,
        [
            "machines_list",
            "image_view",
            "process_run",
            "process_status",
            "process_input",
            "process_cancel",
        ]
    );
    assert!(listed.iter().all(|tool| tool.get("outputSchema").is_some()));
    assert!(listed.iter().all(|tool| !contains_key(tool, "minLength")));
    assert!(listed.iter().all(|tool| !contains_key(tool, "maxLength")));

    let rotated = token(
        &application,
        &[
            ("grant_type", "refresh_token"),
            ("client_id", client),
            ("refresh_token", refresh),
            ("resource", resource),
        ],
    )
    .await?;
    assert_eq!(rotated.status, StatusCode::OK);
    assert_ne!(rotated.body["refresh_token"], refresh);

    let refresh_replay = token(
        &application,
        &[
            ("grant_type", "refresh_token"),
            ("client_id", client),
            ("refresh_token", refresh),
            ("resource", resource),
        ],
    )
    .await?;
    assert_eq!(refresh_replay.status, StatusCode::BAD_REQUEST);

    let wrong_resource = token(
        &application,
        &[
            ("grant_type", "refresh_token"),
            ("client_id", client),
            (
                "refresh_token",
                rotated.body["refresh_token"].as_str().unwrap_or_default(),
            ),
            ("resource", "http://localhost/not-mcp"),
        ],
    )
    .await?;
    assert_eq!(wrong_resource.status, StatusCode::BAD_REQUEST);
    Ok(())
}

struct JsonResponse {
    status: StatusCode,
    body: serde_json::Value,
}

async fn token(application: &axum::Router, pairs: &[(&str, &str)]) -> anyhow::Result<JsonResponse> {
    let form = url::form_urlencoded::Serializer::new(String::new())
        .extend_pairs(pairs.iter().copied())
        .finish();
    let response = application
        .clone()
        .oneshot(
            Request::post("/oauth/token")
                .header(header::CONTENT_TYPE, "application/x-www-form-urlencoded")
                .body(Body::from(form))?,
        )
        .await?;
    let status = response.status();
    let body = response_json(response).await?;
    Ok(JsonResponse { status, body })
}

async fn response_json(response: axum::response::Response) -> anyhow::Result<serde_json::Value> {
    let bytes = to_bytes(response.into_body(), 2 * 1024 * 1024).await?;
    Ok(serde_json::from_slice(&bytes).unwrap_or_else(|_| serde_json::json!({})))
}

fn seed(
    db_path: &std::path::Path,
    client: &str,
    redirect: &str,
    resource: &str,
    verifier: &str,
    code: &str,
) -> anyhow::Result<()> {
    let db = rusqlite::Connection::open(db_path)?;
    db.execute("PRAGMA foreign_keys=ON", [])?;
    let user = "mcp-user";
    let grant_id = "mcp-grant";
    db.execute(
        "INSERT INTO users(id,name,created_at) VALUES(?,?,?)",
        rusqlite::params![user, "MCP User", now_ms()],
    )?;
    db.execute(
        "INSERT INTO mcp_clients(id,name,redirect_uris,created_at) VALUES(?,?,?,?)",
        rusqlite::params![
            client,
            "MCP Test",
            serde_json::to_string(&[redirect])?,
            now_ms()
        ],
    )?;
    let grant = serde_json::to_string(&McpGrantPayload {
        v: 1,
        id: grant_id.into(),
        user_id: user.into(),
        client_id: client.into(),
        client_name: "MCP Test".into(),
        device_ids: Vec::new(),
        scopes: vec!["mcp:observe".into(), "mcp:terminal".into()],
        issued_at: now_ms(),
        expires_at: now_ms() + 60 * 60_000,
    })?;
    db.execute(
        "INSERT INTO mcp_grants(id,user_id,client_id,name,grant,grant_signature,client_control_id,credential_id,control_grant,control_assertion,created_at,expires_at) VALUES(?,?,?,?,?,'signature','control','credential','control-grant','assertion',?,?)",
        rusqlite::params![grant_id,user,client,"MCP Test",grant,now_ms(),now_ms()+60*60_000],
    )?;
    let challenge = URL_SAFE_NO_PAD.encode(Sha256::digest(verifier.as_bytes()));
    db.execute(
        "INSERT INTO oauth_codes(code_hash,grant_id,redirect_uri,code_challenge,resource,expires_at) VALUES(?,?,?,?,?,?)",
        rusqlite::params![hash(code),grant_id,redirect,challenge,resource,now_ms()+60_000],
    )?;
    Ok(())
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
    let root = std::env::temp_dir().join(format!("rc-mcp-oauth-{}", Uuid::new_v4()));
    std::fs::create_dir_all(&root)?;
    Ok(root)
}

fn contains_key(value: &serde_json::Value, key: &str) -> bool {
    match value {
        serde_json::Value::Object(object) => {
            object.contains_key(key) || object.values().any(|value| contains_key(value, key))
        }
        serde_json::Value::Array(values) => values.iter().any(|value| contains_key(value, key)),
        _ => false,
    }
}
