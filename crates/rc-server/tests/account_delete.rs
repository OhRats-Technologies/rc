use axum::{
    body::{Body, to_bytes},
    http::{Request, StatusCode, header},
};
use rc_server::{AppState, Config, DELETED_USER_ID, active_user_count, app, hash, now_ms};
use std::{net::SocketAddr, path::PathBuf};
use tower::ServiceExt;
use uuid::Uuid;

#[tokio::test]
async fn deleting_an_account_preserves_shared_workspaces_and_tombstones_sole_devices()
-> anyhow::Result<()> {
    let root = temp_root()?;
    let db_path = root.join("rc.sqlite3");
    let state = AppState::new(test_config(&root, &db_path))?;
    let owner = "user-delete";
    let survivor = "user-survive";
    let shared = "workspace-shared";
    let sole = "workspace-sole";
    let session = "session-delete";
    let step = "step-delete";
    seed(&db_path, owner, survivor, shared, sole, session, step)?;

    let response = app(state.clone())
        .oneshot(
            Request::delete("/api/v1/account")
                .header(header::COOKIE, format!("rc_session={session}"))
                .header("x-rc-step-up", step)
                .body(Body::empty())?,
        )
        .await?;
    assert_eq!(response.status(), StatusCode::OK);
    assert!(
        response
            .headers()
            .get(header::SET_COOKIE)
            .and_then(|value| value.to_str().ok())
            .is_some_and(|value| value.contains("Max-Age=0"))
    );
    let body = to_bytes(response.into_body(), 1024).await?;
    assert_eq!(
        serde_json::from_slice::<serde_json::Value>(&body)?["ok"],
        true
    );

    let db = rusqlite::Connection::open(&db_path)?;
    assert_eq!(active_user_count(&state)?, 1);
    assert_eq!(
        db.query_row(
            "SELECT created_by FROM workspaces WHERE id=?",
            [shared],
            |row| row.get::<_, String>(0),
        )?,
        survivor
    );
    assert_eq!(
        db.query_row(
            "SELECT count(*) FROM workspaces WHERE id=?",
            [sole],
            |row| row.get::<_, i64>(0),
        )?,
        0
    );
    assert_eq!(
        db.query_row(
            "SELECT count(*) FROM devices WHERE id='device-shared'",
            [],
            |row| row.get::<_, i64>(0),
        )?,
        1
    );
    assert_eq!(
        db.query_row(
            "SELECT identity_public_key FROM revoked_devices WHERE id='device-sole'",
            [],
            |row| row.get::<_, String>(0),
        )?,
        "sole-identity"
    );
    assert_eq!(
        db.query_row(
            "SELECT created_by FROM processes WHERE id='process-shared'",
            [],
            |row| row.get::<_, String>(0),
        )?,
        DELETED_USER_ID
    );
    assert_eq!(
        db.query_row(
            "SELECT created_by FROM workspace_invites WHERE id='invite-shared'",
            [],
            |row| row.get::<_, String>(0),
        )?,
        DELETED_USER_ID
    );
    assert_eq!(
        db.query_row(
            "SELECT created_by FROM enrollment_tokens WHERE id='enrollment-shared'",
            [],
            |row| row.get::<_, String>(0),
        )?,
        DELETED_USER_ID
    );
    assert_eq!(
        db.query_row("SELECT count(*) FROM users WHERE id=?", [owner], |row| row
            .get::<_, i64>(
            0
        ),)?,
        0
    );
    assert_eq!(
        db.query_row(
            "SELECT count(*) FROM sessions WHERE token_hash=?",
            [hash(session)],
            |row| row.get::<_, i64>(0),
        )?,
        0
    );
    Ok(())
}

fn seed(
    db_path: &std::path::Path,
    owner: &str,
    survivor: &str,
    shared: &str,
    sole: &str,
    session: &str,
    step: &str,
) -> anyhow::Result<()> {
    let db = rusqlite::Connection::open(db_path)?;
    db.execute("PRAGMA foreign_keys=ON", [])?;
    for (id, name) in [(owner, "Delete Me"), (survivor, "Survivor")] {
        db.execute(
            "INSERT INTO users(id,name,created_at) VALUES(?,?,?)",
            rusqlite::params![id, name, now_ms()],
        )?;
    }
    for (id, name) in [(shared, "Shared"), (sole, "Sole")] {
        db.execute(
            "INSERT INTO workspaces(id,name,created_by,created_at) VALUES(?,?,?,?)",
            rusqlite::params![id, name, owner, now_ms()],
        )?;
        db.execute(
            "INSERT INTO workspace_members(workspace_id,user_id,role,joined_at) VALUES(?,?,'owner',?)",
            rusqlite::params![id, owner, now_ms()],
        )?;
    }
    db.execute(
        "INSERT INTO workspace_members(workspace_id,user_id,role,joined_at) VALUES(?,?,'owner',?)",
        rusqlite::params![shared, survivor, now_ms()],
    )?;
    insert_device(&db, "device-shared", shared, "shared-identity")?;
    insert_device(&db, "device-sole", sole, "sole-identity")?;
    db.execute(
        "INSERT INTO processes(id,device_id,origin,status,terminal,created_by,created_at) VALUES('process-shared','device-shared','browser','exited',0,?,?)",
        rusqlite::params![owner, now_ms()],
    )?;
    db.execute(
        "INSERT INTO workspace_invites(id,workspace_id,token_hash,role,created_by,created_at,expires_at) VALUES('invite-shared',?,'invite-hash','viewer',?,?,?)",
        rusqlite::params![shared, owner, now_ms(), now_ms() + 60_000],
    )?;
    db.execute(
        "INSERT INTO enrollment_tokens(id,workspace_id,token_hash,created_by,created_at,expires_at) VALUES('enrollment-shared',?,'enrollment-hash',?,?,?)",
        rusqlite::params![shared, owner, now_ms(), now_ms() + 60_000],
    )?;
    db.execute(
        "INSERT INTO sessions(token_hash,user_id,kind,created_at,expires_at) VALUES(?,?,'browser',?,?)",
        rusqlite::params![hash(session), owner, now_ms(), now_ms() + 60_000],
    )?;
    db.execute(
        "INSERT INTO ceremonies(id,kind,user_id,meta_json,state_json,expires_at) VALUES(?,'step-token',?,'{}','{}',?)",
        rusqlite::params![
            hash(step),
            owner,
            now_ms() + 60_000,
        ],
    )?;
    Ok(())
}

fn insert_device(
    db: &rusqlite::Connection,
    id: &str,
    workspace: &str,
    identity: &str,
) -> anyhow::Result<()> {
    db.execute(
        "INSERT INTO devices(id,workspace_id,name,hostname,platform,arch,identity_public_key,transport_public_key,version,capabilities,created_at) VALUES(?,?,?,?,?,?,?,?,?,'[]',?)",
        rusqlite::params![id,workspace,id,"host","darwin","arm64",identity,format!("{identity}-transport"),"0.16.0",now_ms()],
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
    let root = std::env::temp_dir().join(format!("rc-account-delete-{}", Uuid::new_v4()));
    std::fs::create_dir_all(&root)?;
    Ok(root)
}
