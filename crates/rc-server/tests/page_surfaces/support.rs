use axum::{
    body::{Body, to_bytes},
    http::{Request, StatusCode, header},
};
use rc_protocol::McpGrantPayload;
use rc_server::{Config, hash, now_ms};
use std::{net::SocketAddr, path::PathBuf};
use tower::ServiceExt;
use uuid::Uuid;

pub(super) struct SurfaceIds {
    pub(super) user: String,
    pub(super) workspace: String,
    pub(super) leave_workspace: String,
    pub(super) device: String,
    pub(super) session: String,
    pub(super) device_path: String,
    pub(super) process_path: String,
    pub(super) access_path: String,
    pub(super) activity_path: String,
    pub(super) cli_path: String,
}

pub(super) fn seed(db_path: &std::path::Path) -> anyhow::Result<SurfaceIds> {
    let db = rusqlite::Connection::open(db_path)?;
    db.execute("PRAGMA foreign_keys=ON", [])?;
    let user = "surface-user".to_owned();
    let other = "surface-other";
    let workspace = "surface-workspace".to_owned();
    let leave_workspace = "surface-leave".to_owned();
    let device = "surface-device".to_owned();
    let process = "surface-process";
    let session = "surface-session".to_owned();
    for (id, name) in [(user.as_str(), "Surface User"), (other, "Other Owner")] {
        db.execute(
            "INSERT INTO users(id,name,created_at) VALUES(?,?,?)",
            rusqlite::params![id, name, now_ms()],
        )?;
    }
    db.execute(
        "INSERT INTO workspaces(id,name,created_by,created_at) VALUES(?,?,?,?)",
        rusqlite::params![workspace, "Surface Workspace", user, now_ms()],
    )?;
    db.execute(
        "INSERT INTO workspace_members(workspace_id,user_id,role,joined_at) VALUES(?,?,'owner',?)",
        rusqlite::params![workspace, user, now_ms()],
    )?;
    db.execute(
        "INSERT INTO workspaces(id,name,created_by,created_at) VALUES(?,?,?,?)",
        rusqlite::params![leave_workspace, "Leave Workspace", other, now_ms()],
    )?;
    db.execute(
        "INSERT INTO workspace_members(workspace_id,user_id,role,joined_at) VALUES(?,?,'owner',?)",
        rusqlite::params![leave_workspace, other, now_ms()],
    )?;
    db.execute(
        "INSERT INTO workspace_members(workspace_id,user_id,role,joined_at) VALUES(?,?,'operator',?)",
        rusqlite::params![leave_workspace, user, now_ms()],
    )?;
    db.execute(
        "INSERT INTO devices(id,workspace_id,name,hostname,platform,arch,identity_public_key,transport_public_key,version,capabilities,last_seen,created_at) VALUES(?,?,?,?,?,?,?,?,?,'[\"process\",\"webrtc\",\"update\"]',?,?)",
        rusqlite::params![device,workspace,"Surface Mac","surface.local","darwin","arm64","surface-identity","surface-transport","0.16.0",now_ms(),now_ms()],
    )?;
    db.execute(
        "INSERT INTO processes(id,device_id,origin,status,terminal,created_by,created_at,started_at) VALUES(?,?,'browser','running',1,?,?,?)",
        rusqlite::params![process,device,user,now_ms(),now_ms()],
    )?;
    db.execute(
        "INSERT INTO passkeys(id,user_id,name,credential_json,created_at,last_used) VALUES('surface-passkey',?,'Passkey','{}',?,?)",
        rusqlite::params![user,now_ms(),now_ms()],
    )?;
    db.execute(
        "INSERT INTO clients(id,user_id,kind,name,public_key,scopes,created_at,expires_at,last_used) VALUES('surface-api-key',?,'api','Surface API','public','[\"read\",\"execute\"]',?,0,?)",
        rusqlite::params![user,now_ms(),now_ms()],
    )?;
    db.execute(
        "INSERT INTO workspace_invites(id,workspace_id,token_hash,role,created_by,created_at,expires_at) VALUES('surface-invite',?,'surface-invite-hash','viewer',?,?,?)",
        rusqlite::params![workspace,user,now_ms(),now_ms()+60_000],
    )?;
    db.execute(
        "INSERT INTO events(workspace_id,user_id,device_id,kind,detail,created_at) VALUES(?,?,?,'device.renamed','{\"name\":\"Surface Mac\"}',?)",
        rusqlite::params![workspace,user,device,now_ms()],
    )?;
    db.execute(
        "INSERT INTO mcp_clients(id,name,redirect_uris,created_at) VALUES('surface-mcp-client','Surface MCP','[\"http://localhost/callback\"]',?)",
        [now_ms()],
    )?;
    let grant = serde_json::to_string(&McpGrantPayload {
        v: 1,
        id: "surface-mcp-grant".into(),
        user_id: user.clone(),
        client_id: "surface-mcp-client".into(),
        client_name: "Surface MCP".into(),
        device_ids: vec![device.clone()],
        scopes: vec!["mcp:observe".into()],
        issued_at: now_ms(),
        expires_at: now_ms() + 60_000,
    })?;
    db.execute(
        "INSERT INTO mcp_grants(id,user_id,client_id,name,grant,grant_signature,client_control_id,credential_id,control_grant,control_assertion,created_at,expires_at) VALUES('surface-mcp-grant',?,'surface-mcp-client','Surface MCP',?,'signature','control','credential','grant','assertion',?,?)",
        rusqlite::params![user,grant,now_ms(),now_ms()+60_000],
    )?;
    let cli_code = "surface-cli-code";
    db.execute(
        "INSERT INTO cli_authorizations(id,device_code_hash,user_code_hash,client_id,public_key,lifetime,created_at,expires_at) VALUES('surface-cli-auth','device-hash',?,'surface-cli-client-1234','surface-cli-public','30d',?,?)",
        rusqlite::params![hash(cli_code),now_ms(),now_ms()+60_000],
    )?;
    db.execute(
        "INSERT INTO sessions(token_hash,user_id,kind,created_at,expires_at) VALUES(? ,?,'browser',?,?)",
        rusqlite::params![hash(&session),user,now_ms(),now_ms()+60_000],
    )?;
    Ok(SurfaceIds {
        user,
        workspace: workspace.clone(),
        leave_workspace,
        device: device.clone(),
        session,
        device_path: format!("/devices/{device}"),
        process_path: format!("/devices/{device}/processes/{process}"),
        access_path: format!("/workspaces/{workspace}/access"),
        activity_path: format!("/workspaces/{workspace}/activity"),
        cli_path: format!("/cli/login?code={cli_code}"),
    })
}

pub(super) struct HttpResult {
    pub(super) status: StatusCode,
    pub(super) body: String,
    pub(super) location: Option<String>,
}

pub(super) async fn get(
    application: &axum::Router,
    path: &str,
    cookie: Option<&str>,
) -> anyhow::Result<HttpResult> {
    let mut builder = Request::get(path);
    if let Some(cookie) = cookie {
        builder = builder.header(header::COOKIE, cookie);
    }
    send(application, builder.body(Body::empty())?).await
}

pub(super) async fn form(
    application: &axum::Router,
    path: &str,
    cookie: &str,
    body: &str,
) -> anyhow::Result<HttpResult> {
    send(
        application,
        Request::post(path)
            .header(header::COOKIE, cookie)
            .header(header::ORIGIN, "http://localhost")
            .header(header::CONTENT_TYPE, "application/x-www-form-urlencoded")
            .body(Body::from(body.to_owned()))?,
    )
    .await
}

async fn send(application: &axum::Router, request: Request<Body>) -> anyhow::Result<HttpResult> {
    let response = application.clone().oneshot(request).await?;
    let status = response.status();
    let location = response
        .headers()
        .get(header::LOCATION)
        .and_then(|value| value.to_str().ok())
        .map(str::to_owned);
    let body = String::from_utf8(
        to_bytes(response.into_body(), 4 * 1024 * 1024)
            .await?
            .to_vec(),
    )?;
    Ok(HttpResult {
        status,
        body,
        location,
    })
}

pub(super) fn test_config(root: &std::path::Path, db_path: &std::path::Path) -> Config {
    Config {
        listen: SocketAddr::from(([127, 0, 0, 1], 0)),
        data_dir: root.to_path_buf(),
        db_path: db_path.to_path_buf(),
        public_url: "http://localhost".into(),
        static_dir: root.to_path_buf(),
        trust_proxy: false,
        setup_token: Some("surface-setup".into()),
        public_signup: true,
        turnstile_site_key: Some("turnstile-site".into()),
        turnstile_secret_key: Some("turnstile-secret".into()),
        turn_token_id: None,
        turn_api_token: None,
        ssh_daemon_port: 2222,
        ssh_internal_port: 3001,
        mcp_access_ttl_minutes: 15,
        execution_history: rc_server::ExecutionHistory::None,
        execution_history_ttl_hours: 168,
    }
}

pub(super) fn temp_root() -> anyhow::Result<PathBuf> {
    let root = std::env::temp_dir().join(format!("rc-pages-{}", Uuid::new_v4()));
    std::fs::create_dir_all(&root)?;
    Ok(root)
}
