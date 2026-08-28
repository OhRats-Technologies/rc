use axum::{
    body::{Body, to_bytes},
    http::{Request, StatusCode, header},
};
use rc_server::{AppState, Config, app, now_ms};
use std::{net::SocketAddr, path::PathBuf};
use tower::ServiceExt;
use uuid::Uuid;

#[tokio::test]
async fn exact_landing_and_documentation_surfaces_are_preserved() -> anyhow::Result<()> {
    let root = temp_root()?;
    let db_path = root.join("rc.sqlite3");
    let state = AppState::new(test_config(&root, &db_path))?;
    let db = rusqlite::Connection::open(&db_path)?;
    db.execute(
        "INSERT INTO users(id,name,created_at) VALUES('public-user','Public User',?)",
        [now_ms()],
    )?;
    let application = app(state);

    for (path, required) in [
        (
            "/",
            "Remote Control<br/><span class=\"hero-muted\">for your machines.</span>",
        ),
        ("/login", "SIGN IN WITH PASSKEY"),
        ("/signup", "cf-turnstile"),
        ("/docs", "<main class=\"docs-layout\">"),
        ("/docs/principles", "The Node enforces execution authority"),
        ("/docs/security", "Browser and CLI encrypted control"),
        ("/docs/authentication", "Control identities"),
        ("/docs/cli", "rc --help"),
        ("/docs/mcp", "OAuth and Node verification"),
        ("/docs/api", "Canonical payload"),
    ] {
        let response = get(&application, path).await?;
        assert_eq!(response.status, StatusCode::OK, "GET {path}");
        assert!(
            response.body.contains(required),
            "GET {path} missing {required}"
        );
        assert!(
            path == "/"
                || path.starts_with("/docs")
                || response.body.contains("<main class=\"auth-shell\">"),
            "GET {path} missing the auth main landmark"
        );
    }

    let landing = get(&application, "/").await?;
    for required in [
        "<meta name=\"robots\" content=\"index,follow\"/>",
        "<span class=\"logo-text\">RC</span>",
        "href=\"/signup\" class=\"or-button\"",
        "01 Security",
        "<h2>Documentation</h2>",
        "https://ohrats.party/blog",
        "https://assets.ohrats.party/assets/menu.a8b9a29f9ccc.js",
    ] {
        assert!(
            landing.body.contains(required),
            "landing missing {required}"
        );
    }
    assert!(!landing.body.contains("OhRats RC</span>"));
    assert!(!landing.body.contains("01 / SAFETY"));
    assert!(landing.body.contains(&format!(
        "styles.css?v={}-browser2",
        env!("CARGO_PKG_VERSION")
    )));

    let docs = get(&application, "/docs").await?;
    for required in [
        "class=\"docs-sidebar\"",
        "class=\"docs-mobile-catalog\"",
        "class=\"docs-toc\"",
        "Security model",
        "Authentication",
        "Interfaces",
        "copy.js?v=",
    ] {
        assert!(docs.body.contains(required), "docs missing {required}");
    }
    let old_quickstart = get(&application, "/docs/quickstart").await?;
    assert_eq!(old_quickstart.status, StatusCode::PERMANENT_REDIRECT);
    assert_eq!(old_quickstart.location.as_deref(), Some("/docs"));

    let signup = get(&application, "/signup").await?;
    assert!(signup.body.contains("data-sitekey=\"turnstile-site\""));
    assert!(
        signup
            .body
            .contains("https://challenges.cloudflare.com/turnstile/v0/api.js")
    );
    let missing = get(&application, "/docs/not-a-topic").await?;
    assert_eq!(missing.status, StatusCode::NOT_FOUND);
    assert!(missing.body.contains("Documentation not found"));
    let install = get(&application, "/install.sh").await?;
    assert_eq!(install.status, StatusCode::OK);
    assert!(install.body.starts_with("#!/bin/sh"));
    let robots = get(&application, "/robots.txt").await?;
    assert_eq!(robots.status, StatusCode::OK);
    assert!(robots.body.contains("Disallow: /oauth/"));
    Ok(())
}

struct HttpResult {
    status: StatusCode,
    body: String,
    location: Option<String>,
}

async fn get(application: &axum::Router, path: &str) -> anyhow::Result<HttpResult> {
    let response = application
        .clone()
        .oneshot(Request::get(path).body(Body::empty())?)
        .await?;
    let status = response.status();
    let location = response
        .headers()
        .get(header::LOCATION)
        .and_then(|value| value.to_str().ok())
        .map(str::to_owned);
    let body = String::from_utf8(
        to_bytes(response.into_body(), 2 * 1024 * 1024)
            .await?
            .to_vec(),
    )?;
    Ok(HttpResult {
        status,
        body,
        location,
    })
}

fn test_config(root: &std::path::Path, db_path: &std::path::Path) -> Config {
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

fn temp_root() -> anyhow::Result<PathBuf> {
    let root = std::env::temp_dir().join(format!("rc-public-pages-{}", Uuid::new_v4()));
    std::fs::create_dir_all(&root)?;
    Ok(root)
}
