use super::indexed_public_document;

const LOGO: &str = "https://assets.ohrats.party/assets/logo.092a1cece4d0.svg";

pub fn landing() -> String {
    indexed_public_document(
        "Remote control for your machines",
        format!(
            "<div class=\"public-site\">{}<main><header class=\"hero-header\"><div class=\"ohrats-grid\" aria-hidden=\"true\"></div><div class=\"hero-content\"><p class=\"eyebrow\">RC / REMOTE CONTROL</p><h1>Remote control for your machines<br><span class=\"hero-muted\">without exposing SSH.</span></h1><div class=\"public-hero-actions\"><a href=\"/docs\" class=\"or-button\">GET STARTED {}</a><a href=\"/login\" class=\"header-link\">SIGN IN {}</a></div></div></header><section class=\"public-section\"><div class=\"container\"><div class=\"section-grid\"><div class=\"section-heading-stack\"><p class=\"eyebrow\">01 / SAFETY</p><div class=\"section-title-container\"><h2>Private remote control<br><span class=\"muted\">with checks on both ends.</span></h2></div></div><div class=\"public-safety-list\"><div class=\"public-safety-item\"><h3>Passkeys</h3><p>Human sign-in and sensitive approvals use WebAuthn. RC does not use account passwords.</p></div><div class=\"public-safety-item\"><h3>RC Lock</h3><p>Each Node stores workspace authority locally and verifies Owner-signed changes before execution.</p></div><div class=\"public-safety-item\"><h3>Encrypted control</h3><p>Browser and CLI process traffic is encrypted client-to-Node; hosted process history keeps metadata only.</p></div><div class=\"public-safety-item\"><h3>Scoped automation</h3><p>API requests are signed, MCP grants select machines and capabilities, and Node updates require signed releases.</p></div></div></div></div></section><section class=\"public-section\"><div class=\"container\"><header class=\"section-header public-resources-heading\"><div class=\"section-heading-stack\"><p class=\"eyebrow\">02 / RESOURCES</p><div class=\"section-title-container\"><h2>Use RC<br><span class=\"muted\">from the interface you need.</span></h2></div></div><div class=\"public-resource-links\"><a class=\"header-link\" href=\"/docs\">DOCS {}</a><a class=\"header-link\" href=\"/docs/mcp\">MCP {}</a><a class=\"header-link\" href=\"/docs/api\">API {}</a><a class=\"header-link\" href=\"/docs/cli\">CLI {}</a></div></header></div></section></main>{}</div>",
            navigation(),
            arrow(),
            arrow(),
            arrow(),
            arrow(),
            arrow(),
            arrow(),
            footer()
        ),
        &["public"],
        &["public"],
        "<link rel=\"canonical\" href=\"https://rc.ohrats.party/\"><meta property=\"og:type\" content=\"website\"><meta property=\"og:title\" content=\"Remote control for your machines | RC\"><meta property=\"og:description\" content=\"Private remote control for your machines without exposing SSH.\"><meta property=\"og:url\" content=\"https://rc.ohrats.party/\"><meta property=\"og:image\" content=\"https://rc.ohrats.party/assets/social-card.png\"><meta name=\"twitter:card\" content=\"summary_large_image\">",
    )
}

fn navigation() -> String {
    format!(
        "<nav class=\"site-nav\"><div class=\"nav-container\"><div class=\"nav-left\"><a href=\"/\" class=\"logo\"><img src=\"{LOGO}\" alt=\"\" class=\"logo-image\"><span class=\"logo-text\">OhRats RC</span></a><div class=\"desktop-menu\">{} </div></div><div class=\"cta-buttons\"><button class=\"theme-toggle\" data-theme-toggle type=\"button\" aria-label=\"Toggle theme\"></button><a href=\"/login\" class=\"cta-link\">Sign in</a><a href=\"/docs\" class=\"or-button\">GET STARTED {}</a></div><button class=\"mobile-menu-btn\" data-menu-toggle type=\"button\" aria-controls=\"mobile-menu\" aria-expanded=\"false\" aria-label=\"Open menu\"><span aria-hidden=\"true\">☰</span></button></div><div class=\"mobile-menu\" id=\"mobile-menu\">{}<div class=\"mobile-divider\"></div><div class=\"mobile-theme-row\"><span class=\"mobile-theme-label\">Theme</span><button class=\"mobile-theme-btn\" data-theme-toggle type=\"button\" aria-label=\"Toggle theme\"></button></div><a href=\"/login\" class=\"mobile-menu-link\">Sign in</a></div></nav>",
        desktop_resources(),
        arrow(),
        mobile_resources()
    )
}

fn desktop_resources() -> String {
    resources()
        .iter()
        .map(|(number, label, href)| {
            format!(
                "<a href=\"{href}\" class=\"menu-item\"><span class=\"menu-number\">{number}</span><span class=\"menu-label\">{label}</span></a>"
            )
        })
        .collect()
}

fn mobile_resources() -> String {
    resources()
        .iter()
        .map(|(number, label, href)| {
            format!("<a href=\"{href}\" class=\"mobile-menu-link\">{number} {label}</a>")
        })
        .collect()
}

fn resources() -> [(&'static str, &'static str, &'static str); 4] {
    [
        ("01", "Docs", "/docs"),
        ("02", "MCP", "/docs/mcp"),
        ("03", "API", "/docs/api"),
        ("04", "CLI", "/docs/cli"),
    ]
}

fn footer() -> String {
    format!(
        "<footer><div class=\"footer-content\"><div class=\"footer-brand\"><a href=\"/\" aria-label=\"OhRats RC home\"><img src=\"{LOGO}\" alt=\"\" class=\"footer-logo\"></a></div><nav class=\"footer-links\"><div class=\"footer-column\"><span class=\"footer-column-label\">Tools</span><a href=\"https://wakebar.ohrats.party\" class=\"footer-link\">WakeBar</a><a href=\"/\" class=\"footer-link\">OhRats RC</a></div><div class=\"footer-column\"><span class=\"footer-column-label\">Resources</span><a href=\"/docs\" class=\"footer-link\">Docs</a><a href=\"https://github.com/OhRats-Technologies\" class=\"footer-link\">GitHub</a></div><div class=\"footer-column\"><span class=\"footer-column-label\">Company</span><a href=\"https://ohrats.party/#research\" class=\"footer-link\">Research</a><a href=\"mailto:contact@ohrats.party\" class=\"footer-link\">Contact</a></div></nav></div><div class=\"footer-bottom\"><span class=\"footer-copyright\">© 2026 OhRats Technologies. All rights reserved.</span></div></footer>"
    )
}

fn arrow() -> &'static str {
    "<svg width=\"12\" height=\"12\" viewBox=\"0 0 24 24\" fill=\"none\" stroke=\"currentColor\" stroke-width=\"2\" stroke-linecap=\"round\" stroke-linejoin=\"round\" aria-hidden=\"true\"><path d=\"M5 12h14M12 5l7 7-7 7\"></path></svg>"
}
