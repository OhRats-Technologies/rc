use axum::{
    body::Body,
    http::{Request, StatusCode, header},
};
use futures_util::StreamExt;
use rc_server::{AppState, Config, app, hash, now_ms};
use std::{net::SocketAddr, path::PathBuf, time::Duration};
use tokio::time::timeout;
use tower::ServiceExt;
use uuid::Uuid;

#[tokio::test]
async fn browser_event_stream_delivers_default_message_events_for_live_clients()
-> anyhow::Result<()> {
    let root = temp_root()?;
    let db_path = root.join("rc.sqlite3");
    let state = AppState::new(test_config(&root, &db_path))?;
    let session = "events-session";
    seed(&db_path, session)?;
    let response = app(state.clone())
        .oneshot(
            Request::get("/api/v1/events")
                .header(header::COOKIE, format!("rc_session={session}"))
                .body(Body::empty())?,
        )
        .await?;
    assert_eq!(response.status(), StatusCode::OK);
    assert!(
        response
            .headers()
            .get(header::CONTENT_TYPE)
            .and_then(|value| value.to_str().ok())
            .is_some_and(|value| value.starts_with("text/event-stream"))
    );

    state.events.emit(
        &state.db,
        "process.started",
        Some("workspace-events"),
        Some("user-events"),
        Some("device-events"),
        serde_json::json!({"processId":"process-events"}),
    )?;
    let mut stream = response.into_body().into_data_stream();
    let chunk = timeout(Duration::from_secs(2), stream.next())
        .await?
        .ok_or_else(|| anyhow::anyhow!("event stream closed"))??;
    let text = std::str::from_utf8(&chunk)?;
    assert!(text.starts_with("data:"), "unexpected SSE frame: {text:?}");
    assert!(!text.contains("event:"), "custom events bypass onmessage");
    let data = text
        .lines()
        .find_map(|line| line.strip_prefix("data: "))
        .ok_or_else(|| anyhow::anyhow!("missing SSE data"))?;
    let event: serde_json::Value = serde_json::from_str(data)?;
    assert_eq!(event["kind"], "process.started");
    assert_eq!(event["deviceId"], "device-events");
    assert_eq!(event["processId"], "process-events");
    assert_eq!(event["audit"], false);
    let persisted: i64 = rusqlite::Connection::open(&db_path)?.query_row(
        "SELECT count(*) FROM events WHERE kind='process.started'",
        [],
        |row| row.get(0),
    )?;
    assert_eq!(persisted, 0);
    Ok(())
}

fn seed(db_path: &std::path::Path, session: &str) -> anyhow::Result<()> {
    let db = rusqlite::Connection::open(db_path)?;
    db.execute("PRAGMA foreign_keys=ON", [])?;
    db.execute(
        "INSERT INTO users(id,name,created_at) VALUES('user-events','Events',?)",
        [now_ms()],
    )?;
    db.execute(
        "INSERT INTO workspaces(id,name,created_by,created_at) VALUES('workspace-events','Events','user-events',?)",
        [now_ms()],
    )?;
    db.execute(
        "INSERT INTO workspace_members(workspace_id,user_id,role,joined_at) VALUES('workspace-events','user-events','owner',?)",
        [now_ms()],
    )?;
    db.execute(
        "INSERT INTO devices(id,workspace_id,name,hostname,platform,arch,identity_public_key,transport_public_key,version,capabilities,created_at) VALUES('device-events','workspace-events','Events','events','darwin','arm64','events-identity','events-transport','0.16.0','[]',?)",
        [now_ms()],
    )?;
    db.execute(
        "INSERT INTO sessions(token_hash,user_id,kind,created_at,expires_at) VALUES(?,'user-events','browser',?,?)",
        rusqlite::params![hash(session),now_ms(),now_ms()+60_000],
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
    let root = std::env::temp_dir().join(format!("rc-events-{}", Uuid::new_v4()));
    std::fs::create_dir_all(&root)?;
    Ok(root)
}
