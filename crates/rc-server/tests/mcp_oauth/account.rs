use super::*;

async fn call(
    application: &axum::Router,
    name: &str,
    arguments: serde_json::Value,
) -> anyhow::Result<serde_json::Value> {
    let response = application.clone().oneshot(Request::post("/mcp")
        .header(header::CONTENT_TYPE, "application/json")
        .header(header::AUTHORIZATION, "Bearer account-access")
        .header("mcp-name", name)
        .header("mcp-protocol-version", MCP_PROTOCOL_VERSION).header("mcp-method", "tools/call")
        .body(Body::from(serde_json::json!({"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":name,"arguments":arguments}}).to_string()))?).await?;
    assert_eq!(response.status(), StatusCode::OK);
    response_json(response).await
}

fn add_device(db: &rusqlite::Connection, id: &str) -> anyhow::Result<()> {
    db.execute("INSERT INTO devices(id,workspace_id,name,hostname,platform,arch,identity_public_key,transport_public_key,version,created_at) VALUES(?,'workspace',?,'host','windows','amd64',?,'transport','1',1)", rusqlite::params![id,id,id])?;
    Ok(())
}

#[tokio::test]
async fn account_access_tracks_future_devices_and_membership_without_widening_legacy_grants()
-> anyhow::Result<()> {
    let root = temp_root()?;
    let path = root.join("rc.sqlite3");
    let state = AppState::new(test_config(&root, &path))?;
    seed(
        &path,
        "client",
        "http://localhost/callback",
        "http://localhost/mcp",
        &"a".repeat(43),
        "code",
    )?;
    let db = rusqlite::Connection::open(&path)?;
    db.execute_batch(
        "INSERT INTO workspaces VALUES('workspace','Workspace','mcp-user',1);
        INSERT INTO workspace_members VALUES('workspace','mcp-user','owner',1);",
    )?;
    db.execute(
        "INSERT INTO oauth_tokens VALUES(?,'mcp-grant','access',?)",
        rusqlite::params![hash("account-access"), now_ms() + 60_000],
    )?;
    let encoded: String = db.query_row("SELECT grant FROM mcp_grants", [], |row| row.get(0))?;
    let mut grant: McpGrantPayload = serde_json::from_str(&encoded)?;
    grant.device_ids = vec!["original".into()];
    db.execute(
        "UPDATE mcp_grants SET grant=?",
        [serde_json::to_string(&grant)?],
    )?;
    add_device(&db, "original")?;
    let application = app(state.clone());
    add_device(&db, "future")?;
    let legacy = call(&application, "machines_list", serde_json::json!({})).await?;
    assert_eq!(
        legacy["result"]["structuredContent"]["machines"]
            .as_array()
            .unwrap()
            .len(),
        1
    );
    grant.v = 2;
    grant.audience = Some("account".into());
    grant.device_ids.clear();
    db.execute(
        "UPDATE mcp_grants SET grant=?",
        [serde_json::to_string(&grant)?],
    )?;
    let account = call(&application, "machines_list", serde_json::json!({})).await?;
    assert_eq!(
        account["result"]["structuredContent"]["machines"]
            .as_array()
            .unwrap()
            .len(),
        2
    );
    let snapshot: rc_protocol::AuthoritySnapshot =
        serde_json::from_str(&rc_server::canonical_authority(&state.db, "workspace")?)?;
    assert_eq!(snapshot.mcp_grants.len(), 1);
    db.execute("UPDATE workspace_members SET role='operator'", [])?;
    let denied = call(
        &application,
        "process_run",
        serde_json::json!({"deviceId":"future","argv":["whoami"]}),
    )
    .await?;
    assert_eq!(denied["result"]["isError"], true);
    assert!(denied.to_string().contains("Owner access"));
    let snapshot: rc_protocol::AuthoritySnapshot =
        serde_json::from_str(&rc_server::canonical_authority(&state.db, "workspace")?)?;
    assert!(snapshot.mcp_grants.is_empty());
    db.execute("DELETE FROM workspace_members", [])?;
    let removed = call(&application, "machines_list", serde_json::json!({})).await?;
    assert!(
        removed["result"]["structuredContent"]["machines"]
            .as_array()
            .unwrap()
            .is_empty()
    );
    Ok(())
}

#[test]
fn new_consent_is_explicit_account_v2_even_without_online_devices() -> anyhow::Result<()> {
    let root = temp_root()?;
    let path = root.join("rc.sqlite3");
    let state = AppState::new(test_config(&root, &path))?;
    seed(
        &path,
        "client",
        "http://localhost/callback",
        "http://localhost/mcp",
        &"a".repeat(43),
        "code",
    )?;
    let db = rusqlite::Connection::open(&path)?;
    db.execute("INSERT INTO mcp_requests(id,user_id,client_id,redirect_uri,state,scope,code_challenge,resource,created_at,expires_at) VALUES('request','mcp-user','client','http://localhost/callback','','mcp:observe mcp:terminal','challenge','http://localhost/mcp',?,?)", rusqlite::params![now_ms(),now_ms()+60_000])?;
    let user = rc_server::UserIdentity {
        id: "mcp-user".into(),
        name: "User".into(),
    };
    let prepared = rc_server::prepare_oauth_grant(
        &state,
        &user,
        "request",
        "account",
        &["mcp:terminal".into()],
        Some("never"),
    )?;
    let grant: McpGrantPayload = serde_json::from_str(&prepared.grant)?;
    assert_eq!(grant.v, 2);
    assert!(grant.allows_device("not-enrolled-yet"));
    assert!(prepared.signature_payload.starts_with("rc-mcp-grant-v2\n"));
    assert!(
        rc_server::prepare_oauth_grant(
            &state,
            &user,
            "request",
            "",
            &["mcp:terminal".into()],
            None
        )
        .is_err()
    );
    Ok(())
}
