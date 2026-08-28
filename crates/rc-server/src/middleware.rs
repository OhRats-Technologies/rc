use crate::AppState;
use axum::{
    Json,
    extract::{ConnectInfo, Request, State},
    http::{HeaderMap, HeaderName, HeaderValue, StatusCode, header},
    middleware::Next,
    response::{IntoResponse, Response},
};
use dashmap::DashMap;
use std::{net::SocketAddr, sync::Arc, time::Instant};

#[derive(Clone, Default)]
pub struct RateLimiter {
    entries: Arc<DashMap<String, Window>>,
}

#[derive(Clone, Copy)]
struct Window {
    started: Instant,
    count: u32,
}

impl RateLimiter {
    fn check(&self, key: String, limit: u32, seconds: u64) -> Result<(), u64> {
        let now = Instant::now();
        let mut entry = self.entries.entry(key).or_insert(Window {
            started: now,
            count: 0,
        });
        let elapsed = now.duration_since(entry.started).as_secs();
        if elapsed >= seconds {
            *entry = Window {
                started: now,
                count: 1,
            };
            return Ok(());
        }
        if entry.count >= limit {
            return Err(seconds.saturating_sub(elapsed).max(1));
        }
        entry.count += 1;
        drop(entry);
        if self.entries.len() > 4096 {
            self.entries
                .retain(|_, value| now.duration_since(value.started).as_secs() < 300);
        }
        Ok(())
    }
}

pub async fn request_policy(
    State(state): State<AppState>,
    request: Request,
    next: Next,
) -> Response {
    if form_requires_same_origin(request.method(), request.uri().path())
        && !same_origin(&state, request.headers())
    {
        let mut response = (
            StatusCode::FORBIDDEN,
            Json(serde_json::json!({"error":"invalid request origin"})),
        )
            .into_response();
        apply_security_headers(&state, &mut response);
        return response;
    }
    if let Some((bucket, limit, seconds)) = rate_policy(request.uri().path()) {
        let identity = rate_identity(&state, request.headers(), request.extensions());
        if let Err(retry_after) =
            state
                .rate_limits
                .check(format!("{bucket}:{identity}"), limit, seconds)
        {
            let mut response = (
                StatusCode::TOO_MANY_REQUESTS,
                Json(serde_json::json!({"error":"rate limit exceeded"})),
            )
                .into_response();
            response.headers_mut().insert(
                header::RETRY_AFTER,
                HeaderValue::from_str(&retry_after.to_string())
                    .unwrap_or_else(|_| HeaderValue::from_static("60")),
            );
            apply_security_headers(&state, &mut response);
            return response;
        }
    }

    let mut response = next.run(request).await;
    apply_security_headers(&state, &mut response);
    response
}

fn form_requires_same_origin(method: &http::Method, path: &str) -> bool {
    if matches!(
        *method,
        http::Method::GET | http::Method::HEAD | http::Method::OPTIONS
    ) {
        return false;
    }
    path.starts_with("/account/")
        || path == "/workspaces"
        || path.starts_with("/workspaces/")
        || (path.starts_with("/devices/") && path.ends_with("/rename"))
}

fn same_origin(state: &AppState, headers: &HeaderMap) -> bool {
    let expected = match url::Url::parse(&state.config.public_url) {
        Ok(value) => value.origin(),
        Err(_) => return false,
    };
    let supplied = headers
        .get(header::ORIGIN)
        .or_else(|| headers.get(header::REFERER))
        .and_then(|value| value.to_str().ok())
        .and_then(|value| url::Url::parse(value).ok());
    if let Some(value) = supplied {
        return value.origin() == expected;
    }
    headers
        .get("sec-fetch-site")
        .and_then(|value| value.to_str().ok())
        .is_some_and(|value| value.eq_ignore_ascii_case("same-origin"))
}

fn rate_policy(path: &str) -> Option<(&'static str, u32, u64)> {
    if path.starts_with("/api/v1/auth/") {
        return Some(("auth", 40, 60));
    }
    if path == "/api/v1/node/enroll" {
        return Some(("enroll", 20, 60));
    }
    if matches!(path, "/api/v1/node/ice" | "/api/v1/node/connect") {
        return Some(("node-bootstrap", 120, 60));
    }
    if path.starts_with("/oauth/") {
        return Some(("oauth", 80, 60));
    }
    None
}

fn rate_identity(state: &AppState, headers: &HeaderMap, extensions: &http::Extensions) -> String {
    for name in ["x-rc-device", "x-rc-key-id"] {
        if let Some(value) = headers
            .get(name)
            .and_then(|value| value.to_str().ok())
            .filter(|value| !value.is_empty() && value.len() <= 128)
        {
            return value.to_owned();
        }
    }
    if state.config.trust_proxy {
        if let Some(value) = headers
            .get("cf-connecting-ip")
            .and_then(|value| value.to_str().ok())
            .filter(|value| value.parse::<std::net::IpAddr>().is_ok())
        {
            return value.to_owned();
        }
        if let Some(value) = headers
            .get("x-forwarded-for")
            .and_then(|value| value.to_str().ok())
            .and_then(|value| value.split(',').next())
            .map(str::trim)
            .filter(|value| value.parse::<std::net::IpAddr>().is_ok())
        {
            return value.to_owned();
        }
    }
    extensions
        .get::<ConnectInfo<SocketAddr>>()
        .map(|value| value.0.ip().to_string())
        .unwrap_or_else(|| "unknown".into())
}

fn apply_security_headers(state: &AppState, response: &mut Response) {
    let headers = response.headers_mut();
    insert(headers, "x-content-type-options", "nosniff");
    insert(headers, "x-frame-options", "DENY");
    insert(headers, "referrer-policy", "no-referrer");
    insert(headers, "x-permitted-cross-domain-policies", "none");
    insert(headers, "cross-origin-opener-policy", "same-origin");
    insert(
        headers,
        "permissions-policy",
        "camera=(), microphone=(), geolocation=(), payment=(), usb=(), publickey-credentials-get=(self), publickey-credentials-create=(self)",
    );
    insert(
        headers,
        "content-security-policy",
        "default-src 'self'; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; script-src 'self' https://assets.ohrats.party https://challenges.cloudflare.com; style-src 'self' 'unsafe-inline' https://assets.ohrats.party https://fonts.googleapis.com; font-src 'self' data: https://fonts.gstatic.com; img-src 'self' data: https://assets.ohrats.party; connect-src 'self' https://challenges.cloudflare.com; frame-src https://challenges.cloudflare.com; worker-src 'self' blob:",
    );
    if state.config.public_url.starts_with("https://") {
        insert(
            headers,
            "strict-transport-security",
            "max-age=31536000; includeSubDomains",
        );
    }
    if response_is_private(headers) && !headers.contains_key(header::CACHE_CONTROL) {
        headers.insert(header::CACHE_CONTROL, HeaderValue::from_static("no-store"));
    }
}

fn response_is_private(headers: &HeaderMap) -> bool {
    headers
        .get(header::CONTENT_TYPE)
        .and_then(|value| value.to_str().ok())
        .is_some_and(|value| {
            value.starts_with("text/html")
                || value.starts_with("application/json")
                || value.starts_with("text/event-stream")
        })
}

fn insert(headers: &mut HeaderMap, name: &'static str, value: &'static str) {
    headers
        .entry(HeaderName::from_static(name))
        .or_insert_with(|| HeaderValue::from_static(value));
}
