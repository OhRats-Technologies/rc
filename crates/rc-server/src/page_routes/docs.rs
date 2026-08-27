use axum::{
    extract::Path,
    http::StatusCode,
    response::{Html, IntoResponse, Response},
};

pub(super) async fn index() -> Html<String> {
    Html(crate::page_html::public_document(
        "Docs",
        "<main class=\"public-site\"><section class=\"public-section\"><div class=\"container\"><h1>RC documentation</h1><p>RC is a direct encrypted remote-control system. Browser and CLI terminal control use WebRTC directly to the RC Node; the hosted service coordinates identity, authorization and ICE/TURN.</p><div class=\"public-resource-links\"><a class=\"header-link\" href=\"/docs/quickstart\">Quickstart</a><a class=\"header-link\" href=\"/docs/security\">Security</a><a class=\"header-link\" href=\"/docs/cli\">CLI</a><a class=\"header-link\" href=\"/docs/mcp\">MCP</a><a class=\"header-link\" href=\"/docs/api\">API</a></div></div></section></main>".into(),
        &[],
        &["public"],
        "",
    ))
}

pub(super) async fn topic(Path(topic): Path<String>) -> Response {
    let page = match topic.as_str() {
        "quickstart" => Some((
            "Quickstart",
            "Install rc, sign in with rc login, enroll a machine from Devices, then use rc shell DEVICE or rc run DEVICE -- COMMAND.",
        )),
        "security" => Some((
            "Security",
            "Node execution remains authoritative. Browser/CLI control is end-to-end encrypted and WebRTC-only; RC Lock is verified locally on the Node.",
        )),
        "cli" => Some((
            "CLI",
            "Use rc login, rc devices, rc shell DEVICE, rc run DEVICE -- COMMAND, rc ssh-config, and rc service install.",
        )),
        "mcp" => Some((
            "MCP",
            "MCP uses OAuth 2.0 authorization code with PKCE, explicit machine scopes, passkey-backed grants, and bounded hosted command output.",
        )),
        "api" => Some((
            "API",
            "RC automation uses proof-of-possession Ed25519 API keys. Every request signature binds the method, path and query, timestamp, nonce, and SHA-256 body digest. See the repository docs for complete signing examples.",
        )),
        _ => None,
    };
    let Some((heading, text)) = page else {
        return (
            StatusCode::NOT_FOUND,
            Html(crate::page_html::error(404, "Documentation not found")),
        )
            .into_response();
    };
    Html(crate::page_html::public_document(
        heading,
        format!(
            "<main class=\"public-site\"><section class=\"public-section\"><div class=\"container docs-copy\"><p class=\"eyebrow\">RC / DOCUMENTATION</p><h1>{}</h1><p>{}</p><p><a class=\"header-link\" href=\"/docs\">All documentation</a></p></div></section></main>",
            crate::page_html::esc(heading),
            crate::page_html::esc(text)
        ),
        &[],
        &["public"],
        "",
    ))
    .into_response()
}

pub(super) async fn install_script() -> Response {
    (
        [
            ("content-type", "text/x-shellscript; charset=utf-8"),
            ("cache-control", "no-cache"),
        ],
        include_str!("../../../../public/install.sh"),
    )
        .into_response()
}

pub(super) async fn robots() -> &'static str {
    "User-agent: *\nAllow: /\nDisallow: /devices\nDisallow: /account\nDisallow: /api/v1/auth/\nDisallow: /oauth/\nDisallow: /mcp\n"
}
