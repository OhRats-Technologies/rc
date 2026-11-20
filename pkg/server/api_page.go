package server

import (
	"net/http"
	"strings"

	"github.com/OhRats-Technologies/rc/pkg/server/pages"
)

func scopeChip(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "read":
		return `<span class="cap-chip"><span class="ui-icon icon-audit" aria-hidden="true"></span><span>Read</span></span>`
	case "execute":
		return `<span class="cap-chip"><span class="ui-icon icon-terminal" aria-hidden="true"></span><span>Execute</span></span>`
	case "manage-devices":
		return `<span class="cap-chip"><span class="ui-icon icon-devices" aria-hidden="true"></span><span>Manage devices</span></span>`
	case "manage-workspaces":
		return `<span class="cap-chip"><span class="ui-icon icon-folder" aria-hidden="true"></span><span>Manage workspaces</span></span>`
	default:
		return `<span class="cap-chip"><span>` + escape(scope) + `</span></span>`
	}
}

func (s *Server) handleAPIPage(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		http.Redirect(w, req, "/login?next=%2Fapi", http.StatusSeeOther)
		return
	}
	keys, err := s.auth.ListAPIKeys(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "API keys could not be loaded")
		return
	}
	var rows strings.Builder
	if len(keys) == 0 {
		rows.WriteString(`<div class="empty-card"><h2>No API keys yet</h2><p class="page-copy">Create a signing key for CLI automation or scripts. RC stores only the public key and never sees your private key again.</p></div>`)
	}
	for _, key := range keys {
		lastUsed := "Never used"
		if key.LastUsedAt != nil {
			lastUsed = "Last used " + key.LastUsedAt.UTC().Format("Jan 2, 2006")
		}
		expires := "No expiry"
		if !key.ExpiresAt.IsZero() {
			expires = "Expires " + key.ExpiresAt.UTC().Format("Jan 2, 2006")
		}
		var chips strings.Builder
		for _, scope := range key.Scopes {
			chips.WriteString(scopeChip(scope))
		}
		rows.WriteString(`<div class="key-row token-row">` +
			`<div class="key-row-icon"><span class="ui-icon icon-key" aria-hidden="true"></span></div>` +
			`<div class="key-row-body"><div class="key-row-title"><strong>` + escape(key.Name) + `</strong>` +
			`<div class="key-row-dates"><span>` + escape(lastUsed) + `</span><span class="dot-sep" aria-hidden="true">·</span><span>` + escape(expires) + `</span></div></div>` +
			`<div class="cap-chips">` + chips.String() + `</div></div>` +
			`<form method="post" action="/api/tokens/` + escape(key.ID) + `/delete">` +
			`<button class="btn-danger-outline" type="submit">Revoke</button></form></div>`)
	}
	body := `<div class="page"><header class="page-header"><div><p class="eyebrow">ACCESS</p><h1>API keys</h1>` +
		`<p class="page-copy">Ed25519 signing keys for CLI automation, CI pipelines, and scripts.</p></div>` +
		`<div class="page-header-actions"><a class="or-button secondary" href="/docs/api">API docs</a>` +
		`<button class="or-button" type="button" data-api-key-new>New API key</button></div></header>` +
		`<section class="panel-card"><div class="panel-head"><span class="eyebrow">ACTIVE KEYS</span><h2>Signing keys (` + itoa(len(keys)) + `)</h2></div>` +
		`<div class="key-list" id="token-list">` + rows.String() + `</div></section>` +
		apiKeyDialog() + `</div>`
	sidebar := s.renderSidebar(user.ID, user.Name, "/api")
	s.writePage(w, http.StatusOK, pages.Authenticated("API access", sidebar, body,
		[]string{"api-page"}, nil, s.assetsVersion()), "no-store")
}

func apiKeyDialog() string {
	return `<dialog class="form-dialog api-key-dialog" data-api-key-dialog aria-labelledby="api-key-dialog-title">` +
		`<div class="dialog-content" data-api-key-create><h2 id="api-key-dialog-title">New API key</h2>` +
		`<form class="dialog-form" data-api-key-form>` +
		`<label>Name<input name="name" data-api-key-name placeholder="Automation" required maxlength="80"></label>` +
		`<label>Access duration<select name="lifetime"><option value="never">Until revoked</option>` +
		`<option value="1h">1 hour</option><option value="1d">1 day</option><option value="7d">7 days</option>` +
		`<option value="30d">30 days</option><option value="90d">90 days</option><option value="180d">180 days</option>` +
		`<option value="1y">1 year</option></select></label>` +
		`<fieldset class="scope-fields"><legend>Permissions</legend>` +
		`<label><input type="checkbox" name="scope" value="read" checked> Read</label>` +
		`<label><input type="checkbox" name="scope" value="execute" checked> Execute</label>` +
		`<label><input type="checkbox" name="scope" value="manage-devices"> Manage devices</label>` +
		`<label><input type="checkbox" name="scope" value="manage-workspaces"> Manage workspaces</label></fieldset>` +
		`<p class="error" data-api-key-error role="alert"></p>` +
		`<div class="dialog-actions"><button class="or-button secondary" type="button" data-api-key-cancel>Cancel</button>` +
		`<button class="or-button" type="submit">Create</button></div></form></div>` +
		`<div class="dialog-content" data-api-key-result hidden><div class="dialog-title-row">` +
		`<span class="ui-icon icon-key"></span><h2>API signing key created</h2></div>` +
		`<p class="page-copy">Copy this private signing key now. RC stores only its public key and cannot recover it.</p>` +
		`<div class="or-copy-field api-key-secret"><code data-api-key-secret></code>` +
		`<button class="or-copy-button" type="button" aria-label="Copy API signing key" data-api-key-copy>` +
		`<span class="or-copy-icon"></span></button></div>` +
		`<div class="dialog-actions"><button class="or-button" type="button" data-api-key-done>Done</button></div>` +
		`</div></dialog>`
}
