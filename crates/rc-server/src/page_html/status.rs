use super::{PageContext, authenticated_status_document, esc, public_snapshots};

pub(super) const SHARED_STATUS_STYLE: &str =
    "https://assets.ohrats.party/assets/status.3662d6fc2b2e.css";

pub fn authenticated_not_found(context: &PageContext) -> String {
    authenticated_status_document(context, "Page not found", content("Return to RC"))
}

pub fn public_not_found(public_url: &str, public_signup: bool) -> String {
    let mut html = public_snapshots::render(
        public_snapshots::PublicPage::Landing,
        public_signup,
        public_url,
    );
    html = html.replace(
        "<meta name=\"robots\" content=\"index,follow\"/>",
        "<meta name=\"robots\" content=\"noindex,nofollow\"/>",
    );
    replace_between(&mut html, "<title>", "</title>", "Page not found | RC");
    remove_tag(&mut html, "<link rel=\"canonical\"");
    remove_tag(&mut html, "<meta property=\"og:url\"");
    for (from, to) in [
        ("Remote Control | RC", "Page not found | RC"),
        (
            "Private remote control for your machines without exposing SSH.",
            "The page you requested does not exist.",
        ),
    ] {
        html = html.replace(from, to);
    }
    if let Some(head) = html.find("</head>") {
        html.insert_str(
            head,
            &format!("<link rel=\"stylesheet\" href=\"{SHARED_STATUS_STYLE}\"/>"),
        );
    }
    if let (Some(start), Some(end)) = (html.find("<main>"), html.find("</main>")) {
        html.replace_range(
            start..end + "</main>".len(),
            &format!("<main>{}</main>", content("Return to RC")),
        );
    }
    html
}

fn content(action: &str) -> String {
    format!(
        "<section class=\"or-status-page\"><div class=\"ohrats-grid\" aria-hidden=\"true\"></div><div class=\"or-status-content\"><p class=\"or-status-code\">404 / Not found</p><h1 class=\"or-status-title\">Page not found.</h1><p class=\"or-status-copy\">The page you requested does not exist.</p><div class=\"or-status-actions\"><a class=\"or-button\" href=\"/\">{}</a></div></div></section>",
        esc(action)
    )
}

fn replace_between(value: &mut String, open: &str, close: &str, replacement: &str) {
    let Some(start) = value.find(open) else {
        return;
    };
    let content_start = start + open.len();
    let Some(relative_end) = value[content_start..].find(close) else {
        return;
    };
    value.replace_range(content_start..content_start + relative_end, replacement);
}

fn remove_tag(value: &mut String, prefix: &str) {
    let Some(start) = value.find(prefix) else {
        return;
    };
    let Some(relative_end) = value[start..].find("/>") else {
        return;
    };
    value.replace_range(start..start + relative_end + 2, "");
}
