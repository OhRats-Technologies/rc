use crate::{
    AUTH_SCRIPT_PATH,
    ohrats::rc_webui::{shell, types::PublicDocument},
};

pub fn setup(authorized: bool) -> String {
    let content: String = if authorized {
        "<p class=\"muted\">Create the first account with a passkey.</p><form id=\"setup-form\" class=\"auth-form\"><label>Name<input name=\"name\" autocomplete=\"name\" required autofocus maxlength=\"120\"></label><button class=\"or-button\" type=\"submit\">CREATE PASSKEY</button></form>".into()
    } else {
        "<p class=\"page-copy\">Open the setup link for this RC instance.</p>".into()
    };
    auth("Create RC", &content, "", "", authorized)
}

pub fn login(next: &str) -> String {
    let content = "<p class=\"muted\">Use your passkey to sign in.</p><form id=\"login-form\" class=\"auth-form\"><label>Stay signed in for<select name=\"lifetime\"><option value=\"30d\">30 days</option><option value=\"7d\">7 days</option><option value=\"1d\">1 day</option></select></label><button class=\"or-button\" type=\"submit\">SIGN IN WITH PASSKEY</button></form>";
    auth("Sign in", content, next, "", true)
}

fn auth(title: &str, content: &str, next: &str, extra_head: &str, script: bool) -> String {
    let body = format!(
        "<main class=\"auth-shell\"><div class=\"ohrats-grid auth-grid\" aria-hidden=\"true\"></div><div class=\"auth-content\" data-auth-next=\"{}\"><p class=\"eyebrow\">RC / REMOTE CONTROL</p><h1>{}</h1>{content}<p id=\"auth-error\" class=\"error\" role=\"alert\"></p></div></main>",
        escape(next),
        escape(title),
    );
    shell::render_public(&PublicDocument {
        title: title.into(),
        body,
        scripts: if script {
            vec![AUTH_SCRIPT_PATH.into()]
        } else {
            Vec::new()
        },
        styles: Vec::new(),
        extra_head: extra_head.into(),
        indexable: false,
    })
}

fn escape(value: &str) -> String {
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
    fn escapes_authentication_attributes() {
        assert_eq!(escape("/devices?x=\"<&"), "/devices?x=&quot;&lt;&amp;");
    }
}
