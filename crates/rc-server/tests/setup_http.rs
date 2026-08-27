#[allow(dead_code)]
#[path = "passkeys_http/support.rs"]
mod support;

use axum::http::StatusCode;
use rc_server::app;
use support::{get_json, json_request, temp_root, test_state_with_setup_token};

#[tokio::test]
async fn setup_link_cookie_authorizes_setup_api() -> anyhow::Result<()> {
    let root = temp_root()?;
    let application = app(test_state_with_setup_token(&root, Some("setup-secret"))?);

    let denied = json_request(
        &application,
        "/api/v1/auth/setup/options",
        serde_json::json!({"name":"Setup User"}),
        None,
    )
    .await?;
    assert_eq!(denied.status, StatusCode::FORBIDDEN);

    let setup_link = get_json(&application, "/setup/setup-secret", None).await?;
    assert_eq!(setup_link.status, StatusCode::SEE_OTHER);
    let cookie = setup_link.cookie()?;

    let status = get_json(&application, "/api/v1/status", Some(&cookie)).await?;
    assert_eq!(status.status, StatusCode::OK);
    assert_eq!(status.body["setupAuthorized"], true);

    let allowed = json_request(
        &application,
        "/api/v1/auth/setup/options",
        serde_json::json!({"name":"Setup User"}),
        Some(&cookie),
    )
    .await?;
    assert_eq!(allowed.status, StatusCode::OK);
    Ok(())
}

#[tokio::test]
async fn login_before_setup_is_actionable() -> anyhow::Result<()> {
    let root = temp_root()?;
    let application = app(test_state_with_setup_token(&root, Some("setup-secret"))?);

    let login = json_request(
        &application,
        "/api/v1/auth/login/options",
        serde_json::json!({}),
        None,
    )
    .await?;
    assert_eq!(login.status, StatusCode::UNAUTHORIZED);
    assert_eq!(
        login.body["error"],
        "No passkey is registered yet. Complete RC setup first."
    );
    Ok(())
}
