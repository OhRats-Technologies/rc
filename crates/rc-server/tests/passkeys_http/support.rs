use axum::{
    body::{Body, to_bytes},
    http::{Request, StatusCode, header},
};
use rc_server::{AppState, Config};
use std::{net::SocketAddr, path::PathBuf};
use tower::ServiceExt;
use uuid::Uuid;

pub(super) struct JsonResponse {
    pub(super) status: StatusCode,
    pub(super) headers: axum::http::HeaderMap,
    pub(super) body: serde_json::Value,
}

impl JsonResponse {
    pub(super) fn cookie(&self) -> anyhow::Result<String> {
        let value = self
            .headers
            .get(header::SET_COOKIE)
            .and_then(|value| value.to_str().ok())
            .ok_or_else(|| anyhow::anyhow!("missing session cookie"))?;
        Ok(value
            .split(';')
            .next()
            .ok_or_else(|| anyhow::anyhow!("invalid session cookie"))?
            .to_owned())
    }
}

pub(super) async fn json_request(
    application: &axum::Router,
    path: &str,
    body: serde_json::Value,
    cookie: Option<&str>,
) -> anyhow::Result<JsonResponse> {
    json_request_headers(application, path, body, cookie, &[]).await
}

pub(super) async fn json_request_headers(
    application: &axum::Router,
    path: &str,
    body: serde_json::Value,
    cookie: Option<&str>,
    headers: &[(&str, &str)],
) -> anyhow::Result<JsonResponse> {
    let mut builder = Request::post(path).header(header::CONTENT_TYPE, "application/json");
    if let Some(cookie) = cookie {
        builder = builder.header(header::COOKIE, cookie);
    }
    for (name, value) in headers {
        builder = builder.header(*name, *value);
    }
    send(
        application,
        builder.body(Body::from(serde_json::to_vec(&body)?))?,
    )
    .await
}

pub(super) async fn get_json(
    application: &axum::Router,
    path: &str,
    cookie: Option<&str>,
) -> anyhow::Result<JsonResponse> {
    let mut builder = Request::get(path);
    if let Some(cookie) = cookie {
        builder = builder.header(header::COOKIE, cookie);
    }
    send(application, builder.body(Body::empty())?).await
}

pub(super) async fn delete_json(
    application: &axum::Router,
    path: &str,
    cookie: Option<&str>,
    headers: &[(&str, &str)],
) -> anyhow::Result<JsonResponse> {
    let mut builder = Request::delete(path);
    if let Some(cookie) = cookie {
        builder = builder.header(header::COOKIE, cookie);
    }
    for (name, value) in headers {
        builder = builder.header(*name, *value);
    }
    send(application, builder.body(Body::empty())?).await
}

async fn send(application: &axum::Router, request: Request<Body>) -> anyhow::Result<JsonResponse> {
    let response = application.clone().oneshot(request).await?;
    let status = response.status();
    let headers = response.headers().clone();
    let bytes = to_bytes(response.into_body(), 2 * 1024 * 1024).await?;
    let body = serde_json::from_slice(&bytes).unwrap_or_else(|_| serde_json::json!({}));
    Ok(JsonResponse {
        status,
        headers,
        body,
    })
}

pub(super) fn test_state(root: &std::path::Path) -> anyhow::Result<AppState> {
    test_state_with_setup_token(root, None)
}

pub(super) fn test_state_with_setup_token(
    root: &std::path::Path,
    setup_token: Option<&str>,
) -> anyhow::Result<AppState> {
    AppState::new(Config {
        listen: SocketAddr::from(([127, 0, 0, 1], 0)),
        data_dir: root.to_path_buf(),
        db_path: root.join("rc.sqlite3"),
        public_url: "http://localhost".into(),
        static_dir: root.to_path_buf(),
        trust_proxy: false,
        setup_token: setup_token.map(str::to_owned),
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
    })
}

pub(super) fn temp_root() -> anyhow::Result<PathBuf> {
    let root = std::env::temp_dir().join(format!("rc-passkeys-{}", Uuid::new_v4()));
    std::fs::create_dir_all(&root)?;
    Ok(root)
}
