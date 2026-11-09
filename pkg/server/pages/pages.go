// Package pages renders the public and authentication HTML surfaces.
package pages

import (
	"embed"
	"html"
	"strings"
)

//go:embed public_snapshots/*.html
var snapshots embed.FS

// PublicPage selects a public page snapshot.
type PublicPage int

const (
	Landing PublicPage = iota
	Quickstart
	Principles
	Security
	Authentication
	CLI
	MCP
	API
)

// AuthPage selects an authentication page.
type AuthPage int

const (
	Setup AuthPage = iota
	Login
	Signup
	Register
	Join
	InvalidInvite
)

// Options carries the values substituted into rendered pages.
type Options struct {
	PublicURL    string
	Version      string
	PublicSignup bool
	SiteKey      string
	Authorized   bool
	Next         string
	Invite       string
}

var publicFiles = map[PublicPage]string{
	Landing:        "public_snapshots/landing-closed.html",
	Quickstart:     "public_snapshots/quickstart-closed.html",
	Principles:     "public_snapshots/principles-closed.html",
	Security:       "public_snapshots/security-closed.html",
	Authentication: "public_snapshots/authentication-closed.html",
	CLI:            "public_snapshots/cli-closed.html",
	MCP:            "public_snapshots/mcp-closed.html",
	API:            "public_snapshots/api-closed.html",
}

var publicOpenFiles = map[PublicPage]string{
	Landing:        "public_snapshots/landing-open.html",
	Quickstart:     "public_snapshots/quickstart-open.html",
	Principles:     "public_snapshots/principles-open.html",
	Security:       "public_snapshots/security-open.html",
	Authentication: "public_snapshots/authentication-open.html",
	CLI:            "public_snapshots/cli-open.html",
	MCP:            "public_snapshots/mcp-open.html",
	API:            "public_snapshots/api-open.html",
}

// Public returns the snapshot for a public page.
func Public(page PublicPage, options Options) string {
	name := publicFiles[page]
	if options.PublicSignup {
		name = publicOpenFiles[page]
	}
	data, err := snapshots.ReadFile(name)
	if err != nil {
		return ""
	}
	return renderPublic(string(data), options)
}

// Lookup maps a request path to a public page.
func Lookup(path string) (PublicPage, bool) {
	switch path {
	case "/":
		return Landing, true
	case "/docs":
		return Quickstart, true
	case "/docs/principles":
		return Principles, true
	case "/docs/security":
		return Security, true
	case "/docs/authentication":
		return Authentication, true
	case "/docs/cli":
		return CLI, true
	case "/docs/mcp":
		return MCP, true
	case "/docs/api":
		return API, true
	default:
		return Landing, false
	}
}

func renderPublic(document string, options Options) string {
	publicURL := strings.TrimRight(options.PublicURL, "/")
	install := "& ([scriptblock]::Create((Invoke-RestMethod -Uri '" + publicURL + "/install.ps1')))"
	document = strings.ReplaceAll(document, "__WINDOWS_INSTALL_COMMAND__", html.EscapeString(install))
	document = strings.ReplaceAll(document, "__PUBLIC_URL__", html.EscapeString(publicURL))
	document = strings.ReplaceAll(document, "__ASSET_VERSION__", html.EscapeString(options.Version))
	return document
}

// Authenticated wraps a body and sidebar in the shared authenticated shell.
func Authenticated(title, sidebar, body string, scripts []string, version string) string {
	content := `<div class="site-shell">` + sidebar + `<main class="site-content">` + body + `</main></div>`
	scripts = append([]string{"sidebar"}, scripts...)
	document := document(title, content, scripts, nil, "", false, version)
	return strings.Replace(document, "<body>", `<body class="authenticated">`, 1)
}

// Auth returns a complete authentication page.
func Auth(page AuthPage, options Options) string {
	title, content, extraHead := authContent(page, options)
	body := `<main class="auth-shell"><div class="ohrats-grid auth-grid" aria-hidden="true"></div>` +
		`<div class="auth-content" data-auth-next="` + html.EscapeString(options.Next) + `">` +
		`<p class="eyebrow">RC / REMOTE CONTROL</p><h1>` + html.EscapeString(title) + `</h1>` +
		content + `<p id="auth-error" class="error" role="alert"></p></div></main>`
	scripts := []string{"auth"}
	if page == Join || page == InvalidInvite {
		scripts = []string{"pages"}
	}
	return document(title, body, scripts, nil, extraHead, false, options.Version)
}

func authContent(page AuthPage, options Options) (string, string, string) {
	switch page {
	case Setup:
		if options.Authorized {
			return "Create RC",
				`<p class="muted">Create the first account with a passkey.</p>` +
					`<form id="setup-form" class="auth-form"><label>Name<input name="name" autocomplete="name" required autofocus maxlength="120"></label>` +
					`<button class="or-button" type="submit">CREATE PASSKEY</button></form>`, ""
		}
		return "Create RC", `<p class="page-copy">Open the setup link for this RC instance.</p>`, ""
	case Login:
		content := `<p class="muted">Use your passkey to sign in.</p>` +
			`<form id="login-form" class="auth-form"><label>Stay signed in for<select name="lifetime">` +
			`<option value="30d">30 days</option><option value="7d">7 days</option><option value="1d">1 day</option></select></label>` +
			`<button class="or-button" type="submit">SIGN IN WITH PASSKEY</button></form>`
		if options.PublicSignup {
			content += `<a class="text-action" href="/signup">CREATE ACCOUNT</a>`
		}
		return "Sign in", content, ""
	case Signup:
		return "Create account",
			`<p class="muted">Create an RC account with a passkey.</p>` +
				`<form id="signup-form" class="auth-form"><label>Name<input name="name" autocomplete="name" required autofocus maxlength="120"></label>` +
				`<div class="cf-turnstile" data-sitekey="` + html.EscapeString(options.SiteKey) + `" data-action="signup" data-appearance="interaction-only" data-size="flexible"></div>` +
				`<button class="or-button" type="submit">CREATE PASSKEY</button></form>` +
				`<a class="text-action" href="/login">SIGN IN INSTEAD</a>`,
			`<script src="https://challenges.cloudflare.com/turnstile/v0/api.js" async defer></script>`
	case Register:
		return "Join workspace",
			`<p class="muted">Create a passkey to join this workspace.</p>` +
				`<form id="register-form" class="auth-form"><label>Name<input name="name" autocomplete="name" required maxlength="120"></label>` +
				`<input name="invite" type="hidden" value="` + html.EscapeString(options.Invite) + `">` +
				`<button class="or-button" type="submit">CREATE PASSKEY</button></form>`, ""
	case Join:
		return "Join workspace",
			`<p class="muted">Join this workspace with your current account.</p>` +
				`<form data-json-form data-path="/api/v1/workspaces/join" data-redirect="/devices" class="auth-form">` +
				`<input type="hidden" name="token" value="` + html.EscapeString(options.Invite) + `">` +
				`<button class="or-button" type="submit">JOIN WORKSPACE</button></form>`, ""
	default:
		return "Invite unavailable",
			`<p class="muted">This workspace invite is invalid, expired, or already used.</p>` +
				`<a class="text-action" href="/">SIGN IN</a>`, ""
	}
}

func document(title, body string, scripts, styles []string, extraHead string, indexable bool, version string) string {
	var scriptTags, styleTags strings.Builder
	for _, name := range scripts {
		scriptTags.WriteString(`<script type="module" src="/assets/` + html.EscapeString(name) + `.js?v=` + html.EscapeString(version) + `"></script>`)
	}
	for _, name := range styles {
		styleTags.WriteString(`<link rel="stylesheet" href="/assets/` + html.EscapeString(name) + `.css?v=` + html.EscapeString(version) + `">`)
	}
	robots := "noindex,nofollow"
	if indexable {
		robots = "index,follow"
	}
	const base = "https://assets.ohrats.party/assets"
	return `<!doctype html><html lang="en" data-sidebar="open"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="` + robots + `">` +
		`<meta name="color-scheme" content="light dark"><title>` + html.EscapeString(title) + ` | RC</title>` +
		`<meta name="description" content="Persistent terminals and private device access without exposing SSH.">` +
		`<link rel="icon" type="image/svg+xml" href="` + base + `/logo.092a1cece4d0.svg">` +
		`<link rel="stylesheet" href="` + base + `/ohrats.abf84a1e6ae1.css">` +
		`<link rel="stylesheet" href="` + base + `/states.8d99d4b0e704.css">` +
		`<link rel="stylesheet" href="` + base + `/copy.e4c6bbb26b56.css">` +
		`<link rel="preconnect" href="https://fonts.googleapis.com"><link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>` +
		`<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&family=Space+Mono:wght@400;700&display=swap">` +
		`<link rel="stylesheet" href="/assets/styles.css?v=` + html.EscapeString(version) + `">` + extraHead + styleTags.String() +
		`<script src="` + base + `/theme.b6e0fe408633.js"></script></head><body>` + body + scriptTags.String() + `</body></html>`
}
