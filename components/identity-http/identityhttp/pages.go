package identityhttp

import (
	"strings"

	"wit_component/ohrats_rc_webui_shell"
	"wit_component/ohrats_rc_webui_types"
)

const authSetupContent = `<p class="muted">Create the first account with a passkey.</p><form id="setup-form" class="auth-form"><label>Name<input name="name" autocomplete="name" required autofocus maxlength="120"></label><button class="or-button" type="submit">CREATE PASSKEY</button></form>`

const authSetupLockedContent = `<p class="page-copy">Open the setup link for this RC instance.</p>`

const authLoginContent = `<p class="muted">Use your passkey to sign in.</p><form id="login-form" class="auth-form"><label>Stay signed in for<select name="lifetime"><option value="30d">30 days</option><option value="7d">7 days</option><option value="1d">1 day</option></select></label><button class="or-button" type="submit">SIGN IN WITH PASSKEY</button></form>`

func renderSetupPage(authorized bool) string {
	content := authSetupLockedContent
	if authorized {
		content = authSetupContent
	}
	return authPage("Create RC", content, "", "", authorized)
}

func renderLoginPage(next string) string {
	return authPage("Sign in", authLoginContent, next, "", true)
}

func authPage(title string, content string, next string, extraHead string, script bool) string {
	body := `<main class="auth-shell"><div class="ohrats-grid auth-grid" aria-hidden="true"></div><div class="auth-content" data-auth-next="` +
		escapeHTML(next) + `"><p class="eyebrow">RC / REMOTE CONTROL</p><h1>` +
		escapeHTML(title) + `</h1>` + content +
		`<p id="auth-error" class="error" role="alert"></p></div></main>`
	scripts := []string{}
	if script {
		scripts = []string{authScriptPath}
	}
	return ohrats_rc_webui_shell.RenderPublic(ohrats_rc_webui_types.PublicDocument{
		Title:     title,
		Body:      body,
		Scripts:   scripts,
		Styles:    []string{},
		ExtraHead: extraHead,
		Indexable: false,
	})
}

var htmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&#39;",
)

func escapeHTML(value string) string {
	return htmlEscaper.Replace(value)
}
