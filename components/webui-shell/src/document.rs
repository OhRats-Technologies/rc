use crate::CSS_PATH;

const SHARED: &str = "https://assets.ohrats.party/assets";

pub fn public(title: &str, body: &str) -> String {
    format!(
        "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><meta name=\"color-scheme\" content=\"light dark\"><meta name=\"robots\" content=\"noindex,nofollow\"><title>{} | RC</title><meta name=\"description\" content=\"Private remote control for machines you own.\"><link rel=\"icon\" type=\"image/svg+xml\" href=\"{SHARED}/logo.092a1cece4d0.svg\"><link rel=\"stylesheet\" href=\"{SHARED}/ohrats.eb38b77e6b5e.css\"><link rel=\"stylesheet\" href=\"{SHARED}/states.8d99d4b0e704.css\"><link rel=\"stylesheet\" href=\"{CSS_PATH}\"><script src=\"{SHARED}/theme.b6e0fe408633.js\"></script></head><body><header class=\"public-header\"><a class=\"brand\" href=\"/\">RC</a><nav><a href=\"/docs\">Docs</a><a href=\"/login\">Sign in</a></nav></header><main>{body}</main><footer><a href=\"https://ohrats.party/\">OhRats Technologies</a></footer></body></html>",
        escape(title)
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
