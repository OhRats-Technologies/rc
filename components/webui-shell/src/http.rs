use crate::ohrats::rc_http::types::{Header, Request, Response};
use crate::{CSS_PATH, pages};

const CSS: &str = include_str!("../assets/rc.css");

pub fn handle(value: Request) -> Result<Option<Response>, String> {
    if !matches!(value.method.as_str(), "GET" | "HEAD") {
        return Ok(None);
    }
    let head = value.method == "HEAD";
    if value.path == CSS_PATH {
        return Ok(Some(response(
            200,
            "text/css; charset=utf-8",
            "public, max-age=31536000, immutable",
            CSS.as_bytes(),
            head,
        )));
    }
    if value.path == "/robots.txt" {
        return Ok(Some(response(
            200,
            "text/plain; charset=utf-8",
            "public, max-age=3600",
            b"User-agent: *\nDisallow: /\n",
            head,
        )));
    }
    let Some(page) = pages::render(&value.path) else {
        return Ok(None);
    };
    Ok(Some(response(
        page.status,
        "text/html; charset=utf-8",
        page.cache_control,
        page.body.as_bytes(),
        head,
    )))
}

fn response(
    status: u16,
    content_type: &str,
    cache_control: &str,
    body: &[u8],
    head: bool,
) -> Response {
    Response {
        status,
        headers: vec![
            header("content-type", content_type),
            header("cache-control", cache_control),
            header(
                "content-security-policy",
                "default-src 'self'; style-src 'self' https://assets.ohrats.party https://fonts.googleapis.com; font-src https://fonts.gstatic.com; img-src 'self' https://assets.ohrats.party data:; script-src 'self' https://assets.ohrats.party; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'",
            ),
            header("referrer-policy", "same-origin"),
            header("x-content-type-options", "nosniff"),
            header("x-frame-options", "DENY"),
        ],
        body: if head { Vec::new() } else { body.to_vec() },
    }
}

fn header(name: &str, value: &str) -> Header {
    Header {
        name: name.into(),
        value: value.into(),
    }
}
