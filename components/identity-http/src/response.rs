use crate::ohrats::rc_http::types::{Header, Response};
use serde::Serialize;

pub fn html(status: u16, body: String) -> Response {
    response(
        status,
        "text/html; charset=utf-8",
        "no-store",
        body.into_bytes(),
    )
}

pub fn javascript(body: &'static [u8]) -> Response {
    response(
        200,
        "text/javascript; charset=utf-8",
        "public, max-age=31536000, immutable",
        body.to_vec(),
    )
}

pub fn json(status: u16, value: impl Serialize) -> Result<Response, String> {
    Ok(response(
        status,
        "application/json; charset=utf-8",
        "no-store",
        serde_json::to_vec(&value).map_err(display)?,
    ))
}

pub fn error(status: u16, message: &str) -> Result<Response, String> {
    json(status, serde_json::json!({ "error": message }))
}

pub fn redirect(location: &str) -> Response {
    let mut response = response(303, "text/plain; charset=utf-8", "no-store", Vec::new());
    response.headers.push(header("location", location));
    response
}

pub fn with_cookie(mut response: Response, cookie: String) -> Response {
    response.headers.push(header("set-cookie", &cookie));
    response
}

pub fn session_cookie(token: &str, max_age_seconds: u64, secure: bool) -> String {
    format!(
        "rc_session={token}; Path=/; HttpOnly; SameSite=Lax; Max-Age={max_age_seconds}{}",
        if secure { "; Secure" } else { "" }
    )
}

pub fn clear_session_cookie(secure: bool) -> String {
    session_cookie("", 0, secure)
}

fn response(status: u16, content_type: &str, cache_control: &str, body: Vec<u8>) -> Response {
    Response {
        status,
        headers: vec![
            header("content-type", content_type),
            header("cache-control", cache_control),
            header("x-content-type-options", "nosniff"),
            header("x-frame-options", "DENY"),
            header("referrer-policy", "same-origin"),
            header(
                "content-security-policy",
                "default-src 'self'; style-src 'self' https://assets.ohrats.party https://fonts.googleapis.com; font-src https://fonts.gstatic.com; img-src 'self' https://assets.ohrats.party data:; script-src 'self' https://assets.ohrats.party https://challenges.cloudflare.com; connect-src 'self' https://challenges.cloudflare.com; frame-src https://challenges.cloudflare.com; base-uri 'none'; frame-ancestors 'none'; form-action 'self'",
            ),
        ],
        body,
    }
}

fn header(name: &str, value: &str) -> Header {
    Header {
        name: name.into(),
        value: value.into(),
    }
}

fn display(error: impl std::fmt::Display) -> String {
    error.to_string()
}
