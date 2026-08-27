use super::{esc, public_document};

#[derive(Clone, Copy)]
pub enum AuthPage<'a> {
    Setup { authorized: bool },
    Login { next: &'a str, signup: bool },
    Signup { site_key: &'a str },
    Register { invite: &'a str },
    Join { invite: &'a str },
    InvalidInvite,
}

pub fn auth(page: AuthPage<'_>) -> String {
    let (title, content, extra_head) = match page {
        AuthPage::Setup { authorized } => (
            "Create RC",
            if authorized {
                "<p class=\"muted\">Create the first account with a passkey.</p><form id=\"setup-form\" class=\"auth-form\"><label>Name<input name=\"name\" autocomplete=\"name\" required autofocus maxlength=\"120\"></label><button class=\"or-button\" type=\"submit\">CREATE PASSKEY</button></form>".into()
            } else {
                "<p class=\"page-copy\">Open the setup link for this RC instance.</p>".into()
            },
            String::new(),
        ),
        AuthPage::Login { next: _, signup } => (
            "Sign in",
            format!("<p class=\"muted\">Use your passkey to sign in.</p><form id=\"login-form\" class=\"auth-form\"><label>Stay signed in for<select name=\"lifetime\"><option value=\"30d\">30 days</option><option value=\"7d\">7 days</option><option value=\"1d\">1 day</option></select></label><button class=\"or-button\" type=\"submit\">SIGN IN WITH PASSKEY</button></form>{}",if signup{"<a class=\"text-action\" href=\"/signup\">CREATE ACCOUNT</a>"}else{""}),
            String::new(),
        ),
        AuthPage::Signup { site_key } => (
            "Create account",
            format!("<p class=\"muted\">Create an RC account with a passkey.</p><form id=\"signup-form\" class=\"auth-form\"><label>Name<input name=\"name\" autocomplete=\"name\" required autofocus maxlength=\"120\"></label><div class=\"cf-turnstile\" data-sitekey=\"{}\" data-action=\"signup\" data-appearance=\"interaction-only\" data-size=\"flexible\"></div><button class=\"or-button\" type=\"submit\">CREATE PASSKEY</button></form><a class=\"text-action\" href=\"/login\">SIGN IN INSTEAD</a>",esc(site_key)),
            "<script src=\"https://challenges.cloudflare.com/turnstile/v0/api.js\" async defer></script>".into(),
        ),
        AuthPage::Register { invite } => (
            "Join workspace",
            format!("<p class=\"muted\">Create a passkey to join this workspace.</p><form id=\"register-form\" class=\"auth-form\"><label>Name<input name=\"name\" autocomplete=\"name\" required maxlength=\"120\"></label><input name=\"invite\" type=\"hidden\" value=\"{}\"><button class=\"or-button\" type=\"submit\">CREATE PASSKEY</button></form>",esc(invite)),
            String::new(),
        ),
        AuthPage::Join { invite } => (
            "Join workspace",
            format!("<p class=\"muted\">Join this workspace with your current account.</p><form data-json-form data-path=\"/api/v1/workspaces/join\" data-redirect=\"/devices\" class=\"auth-form\"><input type=\"hidden\" name=\"token\" value=\"{}\"><button class=\"or-button\" type=\"submit\">JOIN WORKSPACE</button></form>",esc(invite)),
            String::new(),
        ),
        AuthPage::InvalidInvite => (
            "Invite unavailable",
            "<p class=\"muted\">This workspace invite is invalid, expired, or already used.</p><a class=\"text-action\" href=\"/\">SIGN IN</a>".into(),
            String::new(),
        ),
    };
    let next = match page {
        AuthPage::Login { next, .. } => next,
        _ => "",
    };
    let body = format!(
        "<main class=\"auth-shell\"><div class=\"ohrats-grid auth-grid\" aria-hidden=\"true\"></div><div class=\"auth-content\" data-auth-next=\"{}\"><p class=\"eyebrow\">RC / REMOTE CONTROL</p><h1>{}</h1>{}<p id=\"auth-error\" class=\"error\" role=\"alert\"></p></div></main>",
        esc(next),
        esc(title),
        content
    );
    let scripts = if matches!(page, AuthPage::Join { .. } | AuthPage::InvalidInvite) {
        vec!["pages"]
    } else {
        vec!["auth"]
    };
    public_document(title, body, &scripts, &[], &extra_head)
}
