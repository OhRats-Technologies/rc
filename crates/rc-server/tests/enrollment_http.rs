use axum::{
    body::{Body, to_bytes},
    http::{Request, StatusCode, header},
};
use rc_node::NodeState;
use rc_server::{AppState, Config, app, hash, now_ms};
use std::{net::SocketAddr, path::PathBuf, time::Duration};
use tokio::time::timeout;
use tower::ServiceExt;
use uuid::Uuid;

#[tokio::test]
async fn enrollment_is_atomic_emits_presence_metadata_and_preserves_retry_tokens()
-> anyhow::Result<()> {
    let root = temp_root()?;
    let db_path = root.join("rc.sqlite3");
    let state = AppState::new(test_config(&root, &db_path))?;
    let user_id = Uuid::new_v4().to_string();
    let workspace_id = Uuid::new_v4().to_string();
    seed_workspace(&db_path, &user_id, &workspace_id)?;
    let application = app(state.clone());
    let mut events = state.events.subscribe();

    let first_token = "enroll_first";
    insert_token(&db_path, &workspace_id, &user_id, first_token)?;
    let first_node = NodeState::generate("unused-first".into());
    let first = enroll(&application, first_token, &first_node, "Mac Studio").await?;
    assert_eq!(first.status, StatusCode::CREATED);
    let device_id = first.body["deviceId"]
        .as_str()
        .ok_or_else(|| anyhow::anyhow!("missing device id"))?;
    let event = timeout(Duration::from_secs(2), events.recv()).await??;
    assert_eq!(event.kind, "device.enrolled");
    assert_eq!(event.workspace_id.as_deref(), Some(workspace_id.as_str()));
    assert_eq!(event.device_id.as_deref(), Some(device_id));
    assert_eq!(event.detail["name"], "Mac Studio");
    assert_eq!(event.detail["platform"], "darwin");

    let connection = rusqlite::Connection::open(&db_path)?;
    let used: Option<i64> = connection.query_row(
        "SELECT used_at FROM enrollment_tokens WHERE token_hash=?",
        [hash(first_token)],
        |row| row.get(0),
    )?;
    assert!(used.is_some());
    let inserted: i64 = connection.query_row(
        "SELECT count(*) FROM devices WHERE id=? AND workspace_id=?",
        rusqlite::params![device_id, workspace_id],
        |row| row.get(0),
    )?;
    assert_eq!(inserted, 1);

    let retry_token = "enroll_retry";
    insert_token(&db_path, &workspace_id, &user_id, retry_token)?;
    let duplicate = enroll(&application, retry_token, &first_node, "Duplicate").await?;
    assert_eq!(duplicate.status, StatusCode::CONFLICT);
    let retry_used: Option<i64> = connection.query_row(
        "SELECT used_at FROM enrollment_tokens WHERE token_hash=?",
        [hash(retry_token)],
        |row| row.get(0),
    )?;
    assert_eq!(retry_used, None, "failed enrollment consumed its token");

    let second_node = NodeState::generate("unused-second".into());
    let retry = enroll(&application, retry_token, &second_node, "Retry Mac").await?;
    assert_eq!(retry.status, StatusCode::CREATED);
    let retry_event = timeout(Duration::from_secs(2), events.recv()).await??;
    assert_eq!(retry_event.kind, "device.enrolled");
    assert_eq!(retry_event.detail["name"], "Retry Mac");

    let invalid_token = "enroll_invalid_capability";
    insert_token(&db_path, &workspace_id, &user_id, invalid_token)?;
    let invalid = request(
        &application,
        serde_json::json!({
            "token":invalid_token,
            "name":"Invalid",
            "hostname":"invalid.local",
            "platform":"darwin",
            "arch":"arm64",
            "identityPublicKey":second_node.identity_public_key()?,
            "transportPublicKey":second_node.transport_public_key()?,
            "version":"0.16.0",
            "capabilities":["INVALID CAPABILITY"],
        }),
    )
    .await?;
    assert_eq!(invalid.status, StatusCode::BAD_REQUEST);
    let invalid_used: Option<i64> = connection.query_row(
        "SELECT used_at FROM enrollment_tokens WHERE token_hash=?",
        [hash(invalid_token)],
        |row| row.get(0),
    )?;
    assert_eq!(invalid_used, None);
    Ok(())
}

struct JsonResponse {
    status: StatusCode,
    body: serde_json::Value,
}

async fn enroll(
    application: &axum::Router,
    token: &str,
    node: &NodeState,
    name: &str,
) -> anyhow::Result<JsonResponse> {
    request(
        application,
        serde_json::json!({
            "token":token,
            "name":name,
            "hostname":"test.local",
            "platform":"darwin",
            "arch":"arm64",
            "identityPublicKey":node.identity_public_key()?,
            "transportPublicKey":node.transport_public_key()?,
            "version":"0.16.0",
            "capabilities":["process","webrtc","update"],
        }),
    )
    .await
}

async fn request(
    application: &axum::Router,
    body: serde_json::Value,
) -> anyhow::Result<JsonResponse> {
    let response = application
        .clone()
        .oneshot(
            Request::post("/api/v1/node/enroll")
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::from(serde_json::to_vec(&body)?))?,
        )
        .await?;
    let status = response.status();
    let bytes = to_bytes(response.into_body(), 2 * 1024 * 1024).await?;
    let body = serde_json::from_slice(&bytes).unwrap_or_else(|_| serde_json::json!({}));
    Ok(JsonResponse { status, body })
}

fn seed_workspace(db_path: &std::path::Path, user: &str, workspace: &str) -> anyhow::Result<()> {
    let db = rusqlite::Connection::open(db_path)?;
    db.execute("PRAGMA foreign_keys=ON", [])?;
    db.execute(
        "INSERT INTO users(id,name,created_at) VALUES(?,?,?)",
        rusqlite::params![user, "Owner", now_ms()],
    )?;
    db.execute(
        "INSERT INTO workspaces(id,name,created_by,created_at) VALUES(?,?,?,?)",
        rusqlite::params![workspace, "Test", user, now_ms()],
    )?;
    db.execute(
        "INSERT INTO workspace_members(workspace_id,user_id,role,joined_at) VALUES(?,?,'owner',?)",
        rusqlite::params![workspace, user, now_ms()],
    )?;
    Ok(())
}

fn insert_token(
    db_path: &std::path::Path,
    workspace: &str,
    user: &str,
    token: &str,
) -> anyhow::Result<()> {
    rusqlite::Connection::open(db_path)?.execute(
        "INSERT INTO enrollment_tokens(id,workspace_id,token_hash,created_by,created_at,expires_at) VALUES(?,?,?,?,?,?)",
        rusqlite::params![Uuid::new_v4().to_string(),workspace,hash(token),user,now_ms(),now_ms()+60_000],
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
    }
}

fn temp_root() -> anyhow::Result<PathBuf> {
    let root = std::env::temp_dir().join(format!("rc-enroll-{}", Uuid::new_v4()));
    std::fs::create_dir_all(&root)?;
    Ok(root)
}
