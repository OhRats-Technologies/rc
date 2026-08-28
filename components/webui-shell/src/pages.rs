use crate::{PAGES, document};

pub struct RenderedPage {
    pub status: u16,
    pub cache_control: &'static str,
    pub body: String,
}

pub fn render(path: &str) -> Option<RenderedPage> {
    let (title, body, cache) = match path {
        "/" => (
            "Remote control",
            landing(),
            "public, max-age=0, must-revalidate",
        ),
        "/login" => ("Sign in", login(), "no-store"),
        "/setup" => ("Set up RC", setup(), "no-store"),
        "/docs" => (
            "Documentation",
            docs(),
            "public, max-age=0, must-revalidate",
        ),
        "/docs/cli" => (
            "CLI",
            article(
                "CLI",
                "Install RC, sign in with a passkey, then use rc devices, rc shell, and rc run.",
            ),
            "public, max-age=0, must-revalidate",
        ),
        "/docs/api" => (
            "API",
            article(
                "API",
                "Automation clients use scoped proof-of-possession signing keys. Browser sessions administer those keys.",
            ),
            "public, max-age=0, must-revalidate",
        ),
        "/docs/mcp" => (
            "MCP",
            article(
                "MCP",
                "RC exposes a focused machine/process tool surface through OAuth and explicit machine grants.",
            ),
            "public, max-age=0, must-revalidate",
        ),
        _ => return registered(path),
    };
    Some(RenderedPage {
        status: 200,
        cache_control: cache,
        body: document::public(title, &body),
    })
}

fn registered(path: &str) -> Option<RenderedPage> {
    PAGES.with(|pages| {
        pages
            .borrow()
            .values()
            .find(|page| page.path == path)
            .map(|page| {
                let body = format!(
                    "<article class=\"page\"><p class=\"eyebrow\">RC</p><h1>{}</h1><p>{}</p><pre>{}</pre></article>",
                    document::escape(&page.title),
                    document::escape(&page.summary),
                    document::escape(&page.content)
                );
                RenderedPage {
                    status: 200,
                    cache_control: "no-store",
                    body: document::public(&page.title, &body),
                }
            })
    })
}

fn landing() -> String {
    "<section class=\"hero\"><p class=\"eyebrow\">REMOTE CONTROL</p><h1>Your machines, without opening SSH to the Internet.</h1><p>RC coordinates passkey-backed access while encrypted terminal sessions connect directly to the Node whenever possible.</p><div class=\"actions\"><a class=\"or-button\" href=\"/login\">SIGN IN</a><a href=\"/docs\">READ THE DOCS →</a></div></section><section class=\"features\"><article><h2>Private by default</h2><p>Human terminal traffic remains end-to-end encrypted between controller and Node.</p></article><article><h2>One runtime</h2><p>Browser, CLI, SSH, API, and MCP access share the same authority model.</p></article><article><h2>Composable</h2><p>The RC kernel loads capability-scoped WebAssembly components independently.</p></article></section>".into()
}

fn login() -> String {
    "<section class=\"auth\"><p class=\"eyebrow\">RC</p><h1>Sign in</h1><p>Use a passkey registered with this RC instance.</p><form method=\"post\" action=\"/api/v1/auth/start\"><button class=\"or-button\" type=\"submit\">CONTINUE WITH PASSKEY</button></form></section>".into()
}

fn setup() -> String {
    "<section class=\"auth\"><p class=\"eyebrow\">FIRST ACCOUNT</p><h1>Set up RC</h1><p>Create the first passkey-backed owner account using the protected setup URL supplied by the operator.</p></section>".into()
}

fn docs() -> String {
    "<article class=\"page\"><p class=\"eyebrow\">DOCUMENTATION</p><h1>RC reference</h1><ul class=\"doc-list\"><li><a href=\"/docs/cli\">CLI</a></li><li><a href=\"/docs/api\">API</a></li><li><a href=\"/docs/mcp\">MCP</a></li></ul></article>".into()
}

fn article(title: &str, text: &str) -> String {
    format!(
        "<article class=\"page\"><p class=\"eyebrow\">DOCUMENTATION</p><h1>{}</h1><p>{}</p></article>",
        document::escape(title),
        document::escape(text)
    )
}
