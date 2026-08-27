use super::{PageContext, esc, sidebar};

const SHARED_BASE: &str = "https://assets.ohrats.party/assets";

pub fn public_document(
    title: &str,
    body: String,
    scripts: &[&str],
    styles: &[&str],
    extra_head: &str,
) -> String {
    document(title, body, None, scripts, styles, extra_head, false)
}

pub fn authenticated_document(
    context: &PageContext,
    title: &str,
    body: String,
    scripts: &[&str],
    styles: &[&str],
) -> String {
    document(title, body, Some(context), scripts, styles, "", false)
}

fn document(
    title: &str,
    body: String,
    context: Option<&PageContext>,
    scripts: &[&str],
    styles: &[&str],
    extra_head: &str,
    indexable: bool,
) -> String {
    let mut script_names = Vec::new();
    if context.is_some() {
        script_names.push("sidebar");
    }
    for name in scripts {
        if !script_names.contains(name) {
            script_names.push(name);
        }
    }
    let scripts = script_names
        .into_iter()
        .map(|name| {
            format!(
                "<script type=\"module\" src=\"/assets/{}.js\"></script>",
                esc(name)
            )
        })
        .collect::<String>();
    let styles = styles
        .iter()
        .map(|name| {
            format!(
                "<link rel=\"stylesheet\" href=\"/assets/{}.css\">",
                esc(name)
            )
        })
        .collect::<String>();
    let sidebar_state = context
        .map(|value| value.sidebar.as_str())
        .unwrap_or("open");
    let body_class = if context.is_some() {
        " class=\"authenticated\""
    } else {
        ""
    };
    let content = if let Some(context) = context {
        format!(
            "<div class=\"site-shell\">{}<main class=\"site-content\">{}</main></div>",
            sidebar::render(context),
            body
        )
    } else {
        body
    };
    format!(
        "<!doctype html><html lang=\"en\" data-sidebar=\"{}\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><meta name=\"robots\" content=\"{}\"><meta name=\"color-scheme\" content=\"light dark\"><title>{} | RC</title><meta name=\"description\" content=\"Persistent terminals and private device access without exposing SSH.\"><link rel=\"icon\" type=\"image/svg+xml\" href=\"{}/logo.092a1cece4d0.svg\"><link rel=\"stylesheet\" href=\"{}/ohrats.eb38b77e6b5e.css\"><link rel=\"stylesheet\" href=\"{}/states.8d99d4b0e704.css\"><link rel=\"stylesheet\" href=\"{}/copy.e4c6bbb26b56.css\"><link rel=\"preconnect\" href=\"https://fonts.googleapis.com\"><link rel=\"preconnect\" href=\"https://fonts.gstatic.com\" crossorigin><link rel=\"stylesheet\" href=\"https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&family=Space+Mono:wght@400;700&display=swap\"><link rel=\"stylesheet\" href=\"/assets/styles.css\">{}{}<script src=\"{}/theme.b6e0fe408633.js\"></script>{}</head><body{}>{}{}</body></html>",
        esc(sidebar_state),
        if indexable {
            "index,follow"
        } else {
            "noindex,nofollow"
        },
        esc(title),
        SHARED_BASE,
        SHARED_BASE,
        SHARED_BASE,
        SHARED_BASE,
        styles,
        extra_head,
        SHARED_BASE,
        "",
        body_class,
        content,
        scripts,
    )
}
