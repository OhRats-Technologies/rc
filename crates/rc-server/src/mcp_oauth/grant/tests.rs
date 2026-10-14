use super::*;

#[test]
fn approval_returns_long_state_exactly() -> anyhow::Result<()> {
    let root = std::env::temp_dir().join(format!("rc-oauth-state-{}", Uuid::new_v4()));
    std::fs::create_dir_all(&root)?;
    let state = AppState::new(crate::Config {
        data_dir: root.clone(),
        db_path: root.join("rc.sqlite3"),
        public_url: "http://localhost".into(),
        ..crate::Config::from_env()?
    })?;
    let opaque = format!("{} +/%&=?é", "AOtJLdGvd0_-".repeat(128));
    let request = RequestRow {
        client_id: "client".into(),
        client_name: "Gemini".into(),
        redirect_uri: "https://oauth-redirect.googleusercontent.com/r/test?existing=1".into(),
        state: opaque.clone(),
        scope: "mcp:observe".into(),
        code_challenge: "challenge".into(),
        resource: "http://localhost/mcp".into(),
        prepared_grant: None,
    };
    let payload = McpGrantPayload {
        v: 1,
        id: "grant".into(),
        user_id: "user".into(),
        client_id: "client".into(),
        client_name: "Gemini".into(),
        audience: None,
        device_ids: Vec::new(),
        scopes: vec!["mcp:observe".into()],
        issued_at: now_ms(),
        expires_at: now_ms() + 60_000,
    };
    let approved = approved_redirect(&state, &request, &payload, "code".into())?;
    let redirect = url::Url::parse(&approved.redirect)?;
    let pairs = redirect
        .query_pairs()
        .collect::<std::collections::HashMap<_, _>>();
    assert_eq!(pairs["state"], opaque);
    assert_eq!(pairs["existing"], "1");
    assert_eq!(pairs["code"], "code");
    assert_eq!(pairs["iss"], "http://localhost");
    Ok(())
}
