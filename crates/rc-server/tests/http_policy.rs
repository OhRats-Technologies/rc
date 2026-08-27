use axum::{
    body::Body,
    http::{Request, StatusCode, header},
};
use rc_server::{AppState, Config, app};
use std::{net::SocketAddr, path::PathBuf};
use tower::ServiceExt;
use uuid::Uuid;

#[tokio::test]
async fn responses_have_security_cache_and_request_id_headers() -> anyhow::Result<()> {
    let root = temp_root("headers")?;
    std::fs::write(root.join("test.txt"), "asset")?;
    let application = app(test_state(&root, "https://localhost")?);

    let response = application
        .clone()
        .oneshot(Request::get("/api/v1/health").body(Body::empty())?)
        .await?;
    assert_eq!(response.status(), StatusCode::OK);
    assert_eq!(header_value(&response, "x-content-type-options"), "nosniff");
    assert_eq!(header_value(&response, "x-frame-options"), "DENY");
    assert!(header_value(&response, "content-security-policy").contains("frame-ancestors 'none'"));
    assert_eq!(
        header_value(&response, header::CACHE_CONTROL.as_str()),
        "no-store"
    );
    assert!(response.headers().contains_key("x-request-id"));
    assert!(response.headers().contains_key("strict-transport-security"));

    let asset = application
        .oneshot(Request::get("/assets/test.txt").body(Body::empty())?)
        .await?;
    assert_eq!(asset.status(), StatusCode::OK);
    assert_eq!(
        header_value(&asset, header::CACHE_CONTROL.as_str()),
        "public, max-age=0, must-revalidate"
    );
    assert_eq!(header_value(&asset, "x-content-type-options"), "nosniff");
    Ok(())
}

#[tokio::test]
async fn public_auth_routes_are_rate_limited() -> anyhow::Result<()> {
    let root = temp_root("rate")?;
    let application = app(test_state(&root, "http://localhost")?);
    for _ in 0..40 {
        let response = application
            .clone()
            .oneshot(
                Request::post("/api/v1/auth/login/options")
                    .header(header::CONTENT_TYPE, "application/json")
                    .body(Body::empty())?,
            )
            .await?;
        assert_ne!(response.status(), StatusCode::TOO_MANY_REQUESTS);
    }
    let response = application
        .oneshot(
            Request::post("/api/v1/auth/login/options")
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::empty())?,
        )
        .await?;
    assert_eq!(response.status(), StatusCode::TOO_MANY_REQUESTS);
    assert!(response.headers().contains_key(header::RETRY_AFTER));
    Ok(())
}

#[tokio::test]
async fn browser_form_routes_require_the_configured_origin() -> anyhow::Result<()> {
    let root = temp_root("origin")?;
    let application = app(test_state(&root, "https://localhost")?);
    let rejected = application
        .clone()
        .oneshot(
            Request::post("/account/logout")
                .header(header::CONTENT_TYPE, "application/x-www-form-urlencoded")
                .body(Body::empty())?,
        )
        .await?;
    assert_eq!(rejected.status(), StatusCode::FORBIDDEN);

    let accepted = application
        .oneshot(
            Request::post("/account/logout")
                .header(header::CONTENT_TYPE, "application/x-www-form-urlencoded")
                .header(header::ORIGIN, "https://localhost")
                .body(Body::empty())?,
        )
        .await?;
    assert_eq!(accepted.status(), StatusCode::SEE_OTHER);
    Ok(())
}

fn test_state(root: &std::path::Path, public_url: &str) -> anyhow::Result<AppState> {
    AppState::new(Config {
        listen: SocketAddr::from(([127, 0, 0, 1], 0)),
        data_dir: root.to_path_buf(),
        db_path: root.join("rc.sqlite3"),
        public_url: public_url.into(),
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
    })
}

fn temp_root(label: &str) -> anyhow::Result<PathBuf> {
    let root = std::env::temp_dir().join(format!("rc-http-{label}-{}", Uuid::new_v4()));
    std::fs::create_dir_all(&root)?;
    Ok(root)
}

fn header_value<'a>(response: &'a axum::response::Response, name: &str) -> &'a str {
    response
        .headers()
        .get(name)
        .and_then(|value| value.to_str().ok())
        .unwrap_or_default()
}
