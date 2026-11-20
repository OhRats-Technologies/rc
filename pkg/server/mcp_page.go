package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
	"github.com/OhRats-Technologies/rc/pkg/server/pages"
)

func (s *Server) handleMCPPage(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		http.Redirect(w, req, "/login?next=%2Fintegrations%2Fmcp", http.StatusSeeOther)
		return
	}
	grants, err := s.auth.McpGrantsForUser(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "MCP grants are unavailable")
		return
	}
	var rows strings.Builder
	for _, grant := range grants {
		var payload protocol.McpGrantPayload
		_ = json.Unmarshal([]byte(grant.Grant), &payload)
		expiry := "No expiry"
		if !grant.ExpiresAt.IsZero() {
			expiry = "Expires " + grant.ExpiresAt.UTC().Format("Jan 2, 2006")
		}
		var chips strings.Builder
		for _, sc := range payload.Scopes {
			chips.WriteString(scopeChip(sc))
		}
		rows.WriteString(`<div class="key-row"><div class="key-row-icon"><span class="ui-icon icon-api" aria-hidden="true"></span></div>` +
			`<div class="key-row-body"><div class="key-row-title"><strong>` + escape(grant.Name) + `</strong>` +
			`<div class="key-row-dates"><span>` + escape(grant.ClientID) + `</span><span class="dot-sep" aria-hidden="true">·</span><span>` + escape(expiry) + `</span></div></div>` +
			`<div class="cap-chips">` + chips.String() + `</div></div>` +
			`<button class="btn-danger-outline" type="button" data-mcp-revoke="` + escape(grant.ID) + `">Revoke</button></div>`)
	}
	if rows.Len() == 0 {
		rows.WriteString(`<div class="empty-card"><h2>No MCP clients connected</h2><p class="page-copy">Point Codex, Claude Code, Cursor, VS Code, or agy at the endpoint above to authorize an OAuth grant.</p></div>`)
	}
	endpoint := escape(strings.TrimRight(s.cfg.PublicURL, "/")) + `/mcp`
	body := `<div class="page"><header class="page-header"><div><p class="eyebrow">INTEGRATIONS</p><h1>MCP</h1>` +
		`<p class="page-copy">Connect AI agents via OAuth with an Owner-signed RC Lock grant.</p></div>` +
		`<div class="page-header-actions"><a class="or-button secondary" href="/docs/mcp">MCP docs</a></div></header>` +
		`<section class="panel-card"><div class="panel-head"><span class="eyebrow">01 · ENDPOINT</span><h2>Remote MCP server</h2></div>` +
		`<div class="panel-body"><p class="page-copy">Revoking a connection invalidates its tokens immediately and syncs the lock change to affected Nodes.</p>` +
		`<div class="or-copy-field mcp-endpoint-field" data-copy-value="` + endpoint + `"><code>` + endpoint + `</code><button class="or-copy-button copy-value" type="button" aria-label="Copy MCP endpoint"><span class="or-copy-icon" aria-hidden="true"></span></button></div></div></section>` +
		`<section class="panel-card"><div class="panel-head"><span class="eyebrow">02 · AUTHORIZED AGENTS</span><h2>Connected clients (` + itoa(len(grants)) + `)</h2></div><div class="key-list">` + rows.String() + `</div></section>` +
		`<p class="error" data-mcp-page-error role="alert"></p></div>`
	sidebar := s.renderSidebar(user.ID, user.Name, "/integrations/mcp")
	s.writePage(w, http.StatusOK, pages.Authenticated("MCP", sidebar, body,
		[]string{"mcp-page", "copy"}, nil, s.assetsVersion()), "no-store")
}
