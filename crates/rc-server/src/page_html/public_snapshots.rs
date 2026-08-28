use super::esc;

// Canonical render output from the pre-rewrite public pages at b3e2a9b.
// Keep the archived HTML exact; runtime substitutions are limited to origin and asset revision.

#[derive(Clone, Copy)]
pub enum PublicPage {
    Landing,
    Quickstart,
    Principles,
    Security,
    Authentication,
    Cli,
    Mcp,
    Api,
}

pub fn render(page: PublicPage, public_signup: bool, public_url: &str) -> String {
    let template = match (page, public_signup) {
        (PublicPage::Landing, true) => include_str!("public_snapshots/landing-open.html"),
        (PublicPage::Landing, false) => include_str!("public_snapshots/landing-closed.html"),
        (PublicPage::Quickstart, true) => include_str!("public_snapshots/quickstart-open.html"),
        (PublicPage::Quickstart, false) => include_str!("public_snapshots/quickstart-closed.html"),
        (PublicPage::Principles, true) => include_str!("public_snapshots/principles-open.html"),
        (PublicPage::Principles, false) => include_str!("public_snapshots/principles-closed.html"),
        (PublicPage::Security, true) => include_str!("public_snapshots/security-open.html"),
        (PublicPage::Security, false) => include_str!("public_snapshots/security-closed.html"),
        (PublicPage::Authentication, true) => {
            include_str!("public_snapshots/authentication-open.html")
        }
        (PublicPage::Authentication, false) => {
            include_str!("public_snapshots/authentication-closed.html")
        }
        (PublicPage::Cli, true) => include_str!("public_snapshots/cli-open.html"),
        (PublicPage::Cli, false) => include_str!("public_snapshots/cli-closed.html"),
        (PublicPage::Mcp, true) => include_str!("public_snapshots/mcp-open.html"),
        (PublicPage::Mcp, false) => include_str!("public_snapshots/mcp-closed.html"),
        (PublicPage::Api, true) => include_str!("public_snapshots/api-open.html"),
        (PublicPage::Api, false) => include_str!("public_snapshots/api-closed.html"),
    };
    template
        .replace("__PUBLIC_URL__", &esc(public_url.trim_end_matches('/')))
        .replace("__ASSET_VERSION__", asset_revision())
}

pub fn asset_revision() -> &'static str {
    concat!(env!("CARGO_PKG_VERSION"), "-browser2")
}
