#[path = "page_surfaces/support.rs"]
mod support;

use axum::http::StatusCode;
use rc_server::{AppState, app};
use support::{form, get, seed, temp_root, test_config};

#[tokio::test]
async fn login_redirects_to_setup_when_no_user_exists() -> anyhow::Result<()> {
    let root = temp_root()?;
    let db_path = root.join("rc.sqlite3");
    let application = app(AppState::new(test_config(&root, &db_path))?);
    let response = get(&application, "/login", None).await?;
    assert_eq!(response.status, StatusCode::SEE_OTHER);
    assert_eq!(response.location.as_deref(), Some("/"));
    Ok(())
}

#[tokio::test]
async fn public_authenticated_and_form_surfaces_render_and_mutate() -> anyhow::Result<()> {
    let root = temp_root()?;
    let db_path = root.join("rc.sqlite3");
    let state = AppState::new(test_config(&root, &db_path))?;
    let ids = seed(&db_path)?;
    let application = app(state);
    let cookie = format!("rc_session={}", ids.session);
    let oauth_path = "/oauth/authorize?client_id=surface-mcp-client&redirect_uri=http%3A%2F%2Flocalhost%2Fcallback&response_type=code&code_challenge=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&code_challenge_method=S256&resource=http%3A%2F%2Flocalhost%2Fmcp&scope=mcp%3Aobserve+mcp%3Aterminal&state=surface";

    for (path, required) in [
        (
            "/devices",
            vec![
                "class=\"site-shell\"",
                "id=\"device-list\"",
                "sidebar.js",
                "live.js",
            ],
        ),
        (
            ids.device_path.as_str(),
            vec![
                "data-device-page",
                "id=\"open-terminal\"",
                "id=\"process-list\"",
                "device.js",
            ],
        ),
        (
            ids.process_path.as_str(),
            vec![
                "data-process-page",
                "process-terminal.js",
                "process-terminal.css",
                "data-process-interactive=\"true\"",
            ],
        ),
        (
            "/account",
            vec!["data-account-name-view", "id=\"add-passkey\"", "account.js"],
        ),
        (
            "/api",
            vec!["data-api-key-dialog", "id=\"token-list\"", "api-page.js"],
        ),
        ("/integrations/mcp", vec!["data-mcp-revoke", "mcp-page.js"]),
        (
            ids.access_path.as_str(),
            vec![
                "data-authority-workspace",
                "data-authority-sync",
                "<h2>1 person</h2>",
                "role-form",
                "authority.js",
                "pages.js",
            ],
        ),
        (
            ids.activity_path.as_str(),
            vec![
                "data-activity-page",
                "id=\"activity-list\"",
                "DEVICE.RENAMED",
                "live.js",
            ],
        ),
        (
            "/devices/enroll",
            vec![
                "data-enrollment-form",
                "data-enrollment-result",
                "pages.js",
                "live.js",
            ],
        ),
        (
            ids.cli_path.as_str(),
            vec![
                "<main class=\"auth-shell\"",
                "data-cli-client",
                "AUTHORIZE CLI",
                "cli-authorize.js",
            ],
        ),
        (
            oauth_path,
            vec![
                "<main class=\"auth-shell\"",
                "data-mcp-request",
                "Connect Surface MCP",
                "mcp-authorize.js",
            ],
        ),
    ] {
        let response = get(&application, path, Some(&cookie)).await?;
        assert_eq!(
            response.status,
            StatusCode::OK,
            "GET {path}: {}",
            response.body
        );
        for needle in required {
            assert!(
                response.body.contains(needle),
                "GET {path} missing {needle}"
            );
        }
    }

    for (path, required) in [
        ("/", "SIGN IN WITH PASSKEY"),
        ("/login", "SIGN IN WITH PASSKEY"),
        ("/signup", "cf-turnstile"),
        ("/docs", "RC documentation"),
        ("/docs/quickstart", "Install rc"),
        ("/docs/security", "Node execution remains authoritative"),
        ("/docs/cli", "rc shell DEVICE"),
        ("/docs/mcp", "OAuth 2.0"),
    ] {
        let response = get(&application, path, None).await?;
        assert_eq!(response.status, StatusCode::OK, "GET {path}");
        assert!(
            response.body.contains(required),
            "GET {path} missing {required}"
        );
        assert!(
            path.starts_with("/docs") || response.body.contains("<main class=\"auth-shell\">"),
            "GET {path} missing the auth main landmark"
        );
    }
    let signup = get(&application, "/signup", None).await?;
    assert!(signup.body.contains("data-sitekey=\"turnstile-site\""));
    assert!(
        signup
            .body
            .contains("https://challenges.cloudflare.com/turnstile/v0/api.js")
    );

    let missing = get(&application, "/devices/missing-device", Some(&cookie)).await?;
    assert_eq!(missing.status, StatusCode::NOT_FOUND);
    assert!(missing.body.contains("<main class=\"auth-shell\""));
    let missing_docs = get(&application, "/docs/not-a-topic", None).await?;
    assert_eq!(missing_docs.status, StatusCode::NOT_FOUND);
    assert!(missing_docs.body.contains("Documentation not found"));

    let install = get(&application, "/install.sh", None).await?;
    assert_eq!(install.status, StatusCode::OK);
    assert!(install.body.starts_with("#!/bin/sh"));
    let robots = get(&application, "/robots.txt", None).await?;
    assert_eq!(robots.status, StatusCode::OK);
    assert!(robots.body.contains("Disallow: /oauth/"));

    let created = form(
        &application,
        "/workspaces",
        &cookie,
        "name=Created+by+form&next=%2Fdevices",
    )
    .await?;
    assert_eq!(created.status, StatusCode::SEE_OTHER);
    assert_eq!(created.location.as_deref(), Some("/devices"));

    let renamed = form(
        &application,
        &format!("/workspaces/{}/rename", ids.workspace),
        &cookie,
        "name=Renamed+workspace&next=%2Fdevices",
    )
    .await?;
    assert_eq!(renamed.status, StatusCode::SEE_OTHER);

    let device_renamed = form(
        &application,
        &format!("/devices/{}/rename", ids.device),
        &cookie,
        &format!("name=Renamed+Mac&next=%2Fdevices%2F{}", ids.device),
    )
    .await?;
    assert_eq!(device_renamed.status, StatusCode::SEE_OTHER);

    let left = form(
        &application,
        &format!("/workspaces/{}/leave", ids.leave_workspace),
        &cookie,
        "",
    )
    .await?;
    assert_eq!(left.status, StatusCode::SEE_OTHER);

    let db = rusqlite::Connection::open(&db_path)?;
    assert_eq!(
        db.query_row(
            "SELECT count(*) FROM workspaces WHERE name='Created by form'",
            [],
            |row| row.get::<_, i64>(0),
        )?,
        1
    );
    assert_eq!(
        db.query_row(
            "SELECT name FROM workspaces WHERE id=?",
            [&ids.workspace],
            |row| { row.get::<_, String>(0) }
        )?,
        "Renamed workspace"
    );
    assert_eq!(
        db.query_row(
            "SELECT name FROM devices WHERE id=?",
            [&ids.device],
            |row| { row.get::<_, String>(0) }
        )?,
        "Renamed Mac"
    );
    assert_eq!(
        db.query_row(
            "SELECT count(*) FROM workspace_members WHERE workspace_id=? AND user_id=?",
            rusqlite::params![ids.leave_workspace, ids.user],
            |row| row.get::<_, i64>(0),
        )?,
        0
    );
    Ok(())
}
