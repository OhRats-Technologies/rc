use crate::ohrats::rc_http::types::{Header, Request, Response};
use crate::{COPY_SCRIPT_PATH, PUBLIC_STYLES_PATH, SOCIAL_CARD_PATH, STYLES_PATH, config, pages};

const STYLES: &[u8] = include_bytes!("../assets/styles.css");
const PUBLIC_STYLES: &[u8] = include_bytes!("../assets/public.css");
const COPY_SCRIPT: &[u8] = include_bytes!("../assets/copy.js");
const SIDEBAR_SCRIPT: &[u8] = include_bytes!("../assets/sidebar.js");
const SOCIAL_CARD: &[u8] = include_bytes!("../assets/social-card.png");

pub fn handle(value: Request) -> Result<Option<Response>, String> {
    if !matches!(value.method.as_str(), "GET" | "HEAD") {
        return Ok(None);
    }
    let head = value.method == "HEAD";
    if let Some((content_type, body)) = asset(&value.path) {
        return Ok(Some(response(
            200,
            content_type,
            "public, max-age=31536000, immutable",
            body,
            head,
        )));
    }
    if value.path == "/robots.txt" {
        return Ok(Some(response(
            200,
            "text/plain; charset=utf-8",
            "public, max-age=3600",
            b"User-agent: *\nAllow: /\n",
            head,
        )));
    }
    let public_url = config::public_url(&value);
    let Some(page) = pages::render(&value.path, config::public_signup(), &public_url) else {
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

fn asset(path: &str) -> Option<(&'static str, &'static [u8])> {
    match path {
        value if value == STYLES_PATH => Some(("text/css; charset=utf-8", STYLES)),
        value if value == PUBLIC_STYLES_PATH => Some(("text/css; charset=utf-8", PUBLIC_STYLES)),
        value if value == COPY_SCRIPT_PATH => Some(("text/javascript; charset=utf-8", COPY_SCRIPT)),
        value if value == crate::SIDEBAR_SCRIPT_PATH => {
            Some(("text/javascript; charset=utf-8", SIDEBAR_SCRIPT))
        }
        value if value == SOCIAL_CARD_PATH => Some(("image/png", SOCIAL_CARD)),
        _ => None,
    }
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
                "default-src 'self'; style-src 'self' https://assets.ohrats.party https://fonts.googleapis.com; font-src https://assets.ohrats.party https://fonts.gstatic.com; img-src 'self' https://assets.ohrats.party data:; script-src 'self' https://assets.ohrats.party; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'",
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
