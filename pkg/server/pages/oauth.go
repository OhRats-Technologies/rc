package pages

import (
	"html"
	"strings"
)

// MCPConsentOptions carries one pending OAuth consent request.
type MCPConsentOptions struct {
	PublicURL  string
	Version    string
	RequestID  string
	ClientName string
	UserName   string
	Callback   string
	Scopes     []string
	Error      string
}

// MCPConsent renders the explicit account/scope consent page for Remote MCP.
func MCPConsent(options MCPConsentOptions) string {
	var scopeFields strings.Builder
	if contains(options.Scopes, "mcp:observe") {
		scopeFields.WriteString(consentChoice("mcp:observe", "Observe",
			"List granted machines and read process status and output.", true, true))
	}
	if contains(options.Scopes, "mcp:terminal") {
		scopeFields.WriteString(consentChoice("mcp:terminal", "Terminal",
			"Run arbitrary commands and write process input. Command and output plaintext transits RC.",
			false, true))
	}
	errorText := `<p class="error" data-mcp-error role="alert">` + html.EscapeString(options.Error) + `</p>`
	body := `<main class="auth-shell" data-mcp-request="` + html.EscapeString(options.RequestID) + `">` +
		`<div class="ohrats-grid auth-grid" aria-hidden="true"></div>` +
		`<div class="auth-content mcp-consent"><p class="eyebrow">RC / MCP</p>` +
		`<h1>Connect ` + html.EscapeString(options.ClientName) + `</h1>` +
		`<div class="mcp-identity"><span>Signed in as <strong>` + html.EscapeString(options.UserName) +
		`</strong></span><button class="text-button" type="button" data-mcp-switch-account>NOT YOU?</button></div>` +
		`<p class="page-copy">Choose what this AI agent may do with your account. Access includes current ` +
		`and future machines you can access. Terminal requires Owner access.</p>` +
		`<p class="meta">OAUTH CALLBACK · <code>` + html.EscapeString(options.Callback) + `</code></p>` +
		`<noscript><p class="error">Passkey authorization requires JavaScript.</p></noscript>` +
		`<form class="auth-form" method="post" action="/oauth/authorize/approve" data-mcp-form>` +
		`<input type="hidden" name="request_id" value="` + html.EscapeString(options.RequestID) + `">` +
		`<label>Access duration<select name="lifetime"><option value="never">Until revoked</option>` +
		`<option value="1h">1 hour</option><option value="1d">1 day</option><option value="7d">7 days</option>` +
		`<option value="30d">30 days</option><option value="90d">90 days</option>` +
		`<option value="180d">180 days</option><option value="1y">1 year</option></select></label>` +
		`<fieldset class="scope-fields"><legend>Permissions</legend>` + scopeFields.String() + `</fieldset>` +
		`<div class="mcp-consent-actions"><button class="or-button" type="submit">AUTHORIZE</button>` +
		`<button class="or-button secondary" type="submit" formaction="/oauth/authorize/cancel">CANCEL</button></div>` +
		errorText +
		`<p class="muted">Offline machines become available after reconnecting and accepting their permission ` +
		`update. Access can be revoked from RC at any time. OAuth access tokens remain short-lived and rotate ` +
		`independently.</p></form></div></main>`
	return document("Connect "+options.ClientName, body, []string{"mcp-authorize"}, nil, "", false, options.Version)
}

func consentChoice(scope, label, description string, checked, enabled bool) string {
	attributes := `name="scope" value="` + html.EscapeString(scope) + `"`
	if checked {
		attributes += " checked"
	}
	if !enabled {
		attributes += " disabled"
	}
	return `<label class="mcp-choice"><input type="checkbox" ` + attributes + `>` +
		`<span class="mcp-choice-copy"><strong>` + html.EscapeString(label) + `</strong>` +
		`<small>` + html.EscapeString(description) + `</small></span></label>`
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
