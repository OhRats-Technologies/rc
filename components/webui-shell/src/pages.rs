use crate::{PAGES, document};

#[derive(Clone, Copy)]
enum PublicPage {
    Landing,
    Quickstart,
    Principles,
    Security,
    Authentication,
    Cli,
    Mcp,
    Api,
}

pub struct RenderedPage {
    pub status: u16,
    pub cache_control: &'static str,
    pub body: String,
}

pub fn render(path: &str, public_signup: bool, public_url: &str) -> Option<RenderedPage> {
    let page = match path {
        "/" => PublicPage::Landing,
        "/docs" => PublicPage::Quickstart,
        "/docs/principles" => PublicPage::Principles,
        "/docs/security" => PublicPage::Security,
        "/docs/authentication" => PublicPage::Authentication,
        "/docs/cli" => PublicPage::Cli,
        "/docs/mcp" => PublicPage::Mcp,
        "/docs/api" => PublicPage::Api,
        _ => return registered(path),
    };
    Some(RenderedPage {
        status: 200,
        cache_control: "public, max-age=0, must-revalidate",
        body: render_public(page, public_signup, public_url),
    })
}

fn render_public(page: PublicPage, public_signup: bool, public_url: &str) -> String {
    let template = match (page, public_signup) {
        (PublicPage::Landing, true) => include_str!("../assets/public_snapshots/landing-open.html"),
        (PublicPage::Landing, false) => {
            include_str!("../assets/public_snapshots/landing-closed.html")
        }
        (PublicPage::Quickstart, true) => {
            include_str!("../assets/public_snapshots/quickstart-open.html")
        }
        (PublicPage::Quickstart, false) => {
            include_str!("../assets/public_snapshots/quickstart-closed.html")
        }
        (PublicPage::Principles, true) => {
            include_str!("../assets/public_snapshots/principles-open.html")
        }
        (PublicPage::Principles, false) => {
            include_str!("../assets/public_snapshots/principles-closed.html")
        }
        (PublicPage::Security, true) => {
            include_str!("../assets/public_snapshots/security-open.html")
        }
        (PublicPage::Security, false) => {
            include_str!("../assets/public_snapshots/security-closed.html")
        }
        (PublicPage::Authentication, true) => {
            include_str!("../assets/public_snapshots/authentication-open.html")
        }
        (PublicPage::Authentication, false) => {
            include_str!("../assets/public_snapshots/authentication-closed.html")
        }
        (PublicPage::Cli, true) => include_str!("../assets/public_snapshots/cli-open.html"),
        (PublicPage::Cli, false) => include_str!("../assets/public_snapshots/cli-closed.html"),
        (PublicPage::Mcp, true) => include_str!("../assets/public_snapshots/mcp-open.html"),
        (PublicPage::Mcp, false) => include_str!("../assets/public_snapshots/mcp-closed.html"),
        (PublicPage::Api, true) => include_str!("../assets/public_snapshots/api-open.html"),
        (PublicPage::Api, false) => include_str!("../assets/public_snapshots/api-closed.html"),
    };
    let rendered = template
        .replace(
            "__PUBLIC_URL__",
            &document::escape(public_url.trim_end_matches('/')),
        )
        .replace("__ASSET_VERSION__", env!("CARGO_PKG_VERSION"))
        .replace("/assets/styles.css", crate::STYLES_PATH)
        .replace("/assets/public.css", crate::PUBLIC_STYLES_PATH)
        .replace("/assets/copy.js", crate::COPY_SCRIPT_PATH)
        .replace("/assets/social-card.png", crate::SOCIAL_CARD_PATH);
    currentize(page, rendered)
}

fn currentize(page: PublicPage, html: String) -> String {
    match page {
        PublicPage::Cli => html.replace(
            "<tr><td><code>rc update</code></td><td>Check the latest GitHub Node release. If newer, verify/install it and restart the service if installed; otherwise leave the binary and service untouched.</td></tr>",
            "<tr><td><code>rc</code> / <code>rc --help</code></td><td>Show grouped native, kernel, and active component commands.</td></tr><tr><td><code>rc add SPEC</code> / <code>rc remove NAME</code></td><td>Install or remove a managed WebAssembly component.</td></tr><tr><td><code>rc install</code> / <code>rc list</code></td><td>Restore the locked component set or inspect installed components.</td></tr><tr><td><code>rc outdated [NAME...]</code></td><td>Show available managed-component updates.</td></tr><tr><td><code>rc update [NAME...]</code></td><td>Update managed WebAssembly components without replacing the native RC platform.</td></tr><tr><td><code>rc upgrade</code></td><td>Upgrade the native RC platform and refresh its core component bundle.</td></tr>",
        ),
        PublicPage::Principles => html
            .replace("Node releases are isolated from RC deploys", "Native releases are isolated from component updates")
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

fn registered(path: &str) -> Option<RenderedPage> {
    PAGES.with(|pages| {
        pages
            .borrow()
            .values()
            .find(|page| page.path == path)
            .map(|page| RenderedPage {
                status: 200,
                cache_control: "no-store",
                body: document::registered(&page.title, &page.summary, &page.content),
            })
    })
}

#[cfg(test)]
mod tests {
    use super::render;

    #[test]
    fn renders_the_canonical_landing_and_documentation_layout() {
        let landing = render("/", true, "https://rc.ohrats.party").unwrap().body;
        assert!(
            landing.contains(
                "Remote Control<br/><span class=\"hero-muted\">for your machines.</span>"
            )
        );
        assert!(landing.contains("<link rel=\"canonical\" href=\"https://rc.ohrats.party/\"/>"));
        let docs = render("/docs", false, "https://rc.example").unwrap().body;
        for marker in [
            "docs-sidebar",
            "docs-mobile-catalog",
            "docs-toc",
            "Quickstart",
        ] {
            assert!(docs.contains(marker), "missing {marker}");
        }
    }
}
