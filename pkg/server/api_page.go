package server

import (
	"net/http"
	"strings"

	"github.com/OhRats-Technologies/rc/pkg/server/pages"
	"github.com/OhRats-Technologies/rc/pkg/version"
)

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
		rows.WriteString(`<p class="empty-state">No API keys yet.</p>`)
	}
	for _, key := range keys {
		lastUsed := "NEVER USED"
		if key.LastUsedAt != nil {
			lastUsed = "USED " + key.LastUsedAt.UTC().Format("2006-01-02")
		}
		expires := "UNTIL REVOKED"
		if !key.ExpiresAt.IsZero() {
			expires = "EXPIRES " + key.ExpiresAt.UTC().Format("2006-01-02")
		}
		rows.WriteString(`<div class="setting-row token-row"><div class="token-row-main">` +
			`<span class="ui-icon icon-key" aria-hidden="true"></span><div><strong>` +
			escape(key.Name) + `</strong><div class="meta">` +
			escape(strings.ToUpper(strings.Join(key.Scopes, " · "))) + ` · ` +
			escape(lastUsed) + ` · ` + escape(expires) + `</div></div></div>` +
			`<form method="post" action="/api/tokens/` + escape(key.ID) + `/delete">` +
			`<button class="text-button" type="submit">REVOKE</button></form></div>`)
	}
	body := `<div class="page"><header class="page-header"><div><h1>API</h1></div>` +
		`<div class="page-header-actions"><a class="or-button secondary" href="/docs/api">API DOCS</a>` +
		`<button class="header-icon-button" type="button" aria-label="New API key" title="New key" data-api-key-new>` +
		`<span class="ui-icon icon-plus"></span></button></div></header>` +
		`<div class="settings-list" id="token-list">` + rows.String() + `</div>` +
		apiKeyDialog() + `</div>`
	sidebar := s.renderSidebar(user.ID, user.Name, "/api")
	s.writePage(w, http.StatusOK, pages.Authenticated("API access", sidebar, body,
		[]string{"api-page"}, version.Version), "no-store")
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
