use crate::ohrats::rc_webui::types::{AuthenticatedDocument, PublicDocument, SidebarState};
use crate::{PUBLIC_STYLES_PATH, STYLES_PATH};

const SHARED: &str = "https://assets.ohrats.party/assets";

pub fn public(value: PublicDocument) -> String {
    let scripts = value
        .scripts
        .iter()
        .map(|path| format!("<script type=\"module\" src=\"{}\"></script>", escape(path)))
        .collect::<String>();
    let styles = value
        .styles
        .iter()
        .map(|path| format!("<link rel=\"stylesheet\" href=\"{}\">", escape(path)))
        .collect::<String>();
    format!(
        "<!doctype html><html lang=\"en\" data-sidebar=\"open\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><meta name=\"robots\" content=\"{}\"><meta name=\"color-scheme\" content=\"light dark\"><title>{} | RC</title><meta name=\"description\" content=\"Secure remote control with a small native core and hot-swappable WebAssembly components.\"><link rel=\"icon\" type=\"image/svg+xml\" href=\"{SHARED}/logo.092a1cece4d0.svg\"><link rel=\"stylesheet\" href=\"{SHARED}/ohrats.eb38b77e6b5e.css\"><link rel=\"stylesheet\" href=\"{SHARED}/states.8d99d4b0e704.css\"><link rel=\"stylesheet\" href=\"{SHARED}/copy.e4c6bbb26b56.css\"><link rel=\"preconnect\" href=\"https://fonts.googleapis.com\"><link rel=\"preconnect\" href=\"https://fonts.gstatic.com\" crossorigin><link rel=\"stylesheet\" href=\"https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&family=Space+Mono:wght@400;700&display=swap\"><link rel=\"stylesheet\" href=\"{STYLES_PATH}\"><link rel=\"stylesheet\" href=\"{PUBLIC_STYLES_PATH}\">{}{extra}<script src=\"{SHARED}/theme.b6e0fe408633.js\"></script></head><body>{body}{scripts}</body></html>",
        if value.indexable {
            "index,follow"
        } else {
            "noindex,nofollow"
        },
        escape(&value.title),
        styles,
        extra = value.extra_head,
        body = value.body,
        scripts = scripts,
    )
}

pub fn authenticated(value: AuthenticatedDocument, sidebar_additions: &str) -> String {
    let scripts = std::iter::once(crate::SIDEBAR_SCRIPT_PATH.to_owned())
        .chain(value.scripts.iter().cloned())
        .collect::<Vec<_>>();
    let scripts = scripts
        .iter()
        .map(|path| format!("<script type=\"module\" src=\"{}\"></script>", escape(path)))
        .collect::<String>();
    let styles = value
        .styles
        .iter()
        .map(|path| format!("<link rel=\"stylesheet\" href=\"{}\">", escape(path)))
        .collect::<String>();
    let state = match value.sidebar {
        SidebarState::Open => "open",
        SidebarState::Closed => "closed",
    };
    format!(
        "<!doctype html><html lang=\"en\" data-sidebar=\"{state}\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><meta name=\"robots\" content=\"noindex,nofollow\"><meta name=\"color-scheme\" content=\"light dark\"><title>{} | RC</title><meta name=\"description\" content=\"Secure remote control with a small native core and hot-swappable WebAssembly components.\"><link rel=\"icon\" type=\"image/svg+xml\" href=\"{SHARED}/logo.092a1cece4d0.svg\"><link rel=\"stylesheet\" href=\"{SHARED}/ohrats.eb38b77e6b5e.css\"><link rel=\"stylesheet\" href=\"{SHARED}/states.8d99d4b0e704.css\"><link rel=\"stylesheet\" href=\"{SHARED}/copy.e4c6bbb26b56.css\"><link rel=\"preconnect\" href=\"https://fonts.googleapis.com\"><link rel=\"preconnect\" href=\"https://fonts.gstatic.com\" crossorigin><link rel=\"stylesheet\" href=\"https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&family=Space+Mono:wght@400;700&display=swap\"><link rel=\"stylesheet\" href=\"{STYLES_PATH}\">{styles}<script src=\"{SHARED}/theme.b6e0fe408633.js\"></script></head><body class=\"authenticated\"><div class=\"site-shell\">{}<main class=\"site-content\">{}</main></div>{scripts}</body></html>",
        escape(&value.title),
        crate::sidebar::render(&value, sidebar_additions),
        value.trusted_body,
    )
}

pub fn registered(title: &str, summary: &str, content: &str) -> String {
    public(PublicDocument {
        title: title.into(),
        body: format!(
            "<main class=\"page-shell\"><article class=\"page\"><p class=\"eyebrow\">RC</p><h1>{}</h1><p>{}</p><pre>{}</pre></article></main>",
            escape(title),
            escape(summary),
            escape(content)
        ),
        scripts: Vec::new(),
        styles: Vec::new(),
        extra_head: String::new(),
        indexable: false,
    })
}

pub fn escape(value: &str) -> String {
    value
        .replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
        .replace('"', "&quot;")
        .replace('\'', "&#39;")
}

#[cfg(test)]
mod tests {
    use super::escape;
    #[test]
    fn escapes_markup_and_attributes() {
        assert_eq!(escape("<&\"'>"), "&lt;&amp;&quot;&#39;&gt;");
    }
}
