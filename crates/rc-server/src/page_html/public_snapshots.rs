use super::esc;

// Canonical render output from the pre-rewrite public pages at b3e2a9b.
// Archived HTML stays exact; narrowly scoped runtime substitutions keep current RC commands factual.

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
    let rendered = template
        .replace("__PUBLIC_URL__", &esc(public_url.trim_end_matches('/')))
        .replace("__ASSET_VERSION__", asset_revision());
    currentize(page, rendered)
}

pub fn asset_revision() -> &'static str {
    concat!(env!("CARGO_PKG_VERSION"), "-browser2")
}

fn currentize(page: PublicPage, html: String) -> String {
    match page {
        PublicPage::Cli => html.replace(
            "<tr><td><code>rc update</code></td><td>Check the latest GitHub Node release. If newer, verify/install it and restart the service if installed; otherwise leave the binary and service untouched.</td></tr>",
            "<tr><td><code>rc add SPEC</code> / <code>rc remove NAME</code></td><td>Install or remove a managed WebAssembly component.</td></tr><tr><td><code>rc install</code> / <code>rc list</code></td><td>Restore the locked component set or inspect installed components.</td></tr><tr><td><code>rc outdated [NAME...]</code></td><td>Show available managed-component updates.</td></tr><tr><td><code>rc update [NAME...]</code></td><td>Update managed WebAssembly components without replacing the native RC platform.</td></tr><tr><td><code>rc upgrade</code></td><td>Upgrade the native RC platform and refresh its core component bundle.</td></tr>",
        ),
        PublicPage::Principles => html
            .replace(
                "Node releases are isolated from RC deploys",
                "Native releases are isolated from component updates",
            )
            .replace(
                "GitHub Actions builds and publishes version-tagged Node/CLI releases independently of the RC control-plane deployment.</p><p>The installer and updater download GitHub Release assets directly, verify the selected artifact SHA-256 and reported version, and refuse downgrades.",
                "GitHub Actions publishes native RC and kernel artifacts independently from portable WebAssembly components.</p><p><code>rc update</code> changes managed components. <code>rc upgrade</code> verifies GitHub release digests, refreshes the native platform and core component bundle, and refuses native downgrades.",
            ),
        PublicPage::Security => html.replace(
            "GitHub Releases are the Node release trust boundary. The updater reads the published release manifest, verifies the selected artifact SHA-256, verifies the downloaded binary&#x27;s reported version, and refuses downgrades.</p><p>Node releases are published independently of RC runtime deployments, so a normal control-plane deploy does not rebuild or replace Node binaries.",
            "GitHub Releases are the native RC release trust boundary. <code>rc upgrade</code> verifies the selected RC, kernel, and core-component artifact digests, validates the downloaded executables and component graph, and refuses native downgrades.</p><p>Managed WebAssembly components update independently through <code>rc update</code>; a component-only update does not replace the native RC binary or kernel.",
        ),
        _ => html,
    }
}
