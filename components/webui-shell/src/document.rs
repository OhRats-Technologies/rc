use crate::STYLES_PATH;

const SHARED: &str = "https://assets.ohrats.party/assets";

pub fn registered(title: &str, summary: &str, content: &str) -> String {
    format!(
        "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><meta name=\"color-scheme\" content=\"light dark\"><meta name=\"robots\" content=\"noindex,nofollow\"><title>{} | RC</title><link rel=\"icon\" type=\"image/svg+xml\" href=\"{SHARED}/logo.092a1cece4d0.svg\"><link rel=\"stylesheet\" href=\"{SHARED}/ohrats.eb38b77e6b5e.css\"><link rel=\"stylesheet\" href=\"{SHARED}/states.8d99d4b0e704.css\"><link rel=\"stylesheet\" href=\"{STYLES_PATH}\"><script src=\"{SHARED}/theme.b6e0fe408633.js\"></script></head><body><main class=\"page-shell\"><article class=\"page\"><p class=\"eyebrow\">RC</p><h1>{}</h1><p>{}</p><pre>{}</pre></article></main></body></html>",
        escape(title),
        escape(title),
        escape(summary),
        escape(content)
    )
}

pub fn escape(value: &str) -> String {
    value
        .replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
        .replace('"', "&quot;")
        .replace('\'', "&#39;")
}
