package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

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
		expiry := "until revoked"
		if !grant.ExpiresAt.IsZero() {
			expiry = "expires " + grant.ExpiresAt.UTC().Format(time.RFC3339)
		}
		rows.WriteString(`<div class="data-row"><div><strong>` + escape(grant.Name) +
			`</strong>` + metaRow("div", escape(grant.ClientID),
			escape(strings.Join(payload.Scopes, " ")), escape(expiry)) +
			`</div><button class="or-button secondary" type="button" data-mcp-revoke="` +
			escape(grant.ID) + `">Revoke</button></div>`)
	}
	if rows.Len() == 0 {
		rows.WriteString(`<div class="empty-card"><h2>No MCP clients connected</h2><p class="page-copy">Point Codex, Claude Code, Cursor, VS Code, or agy at the endpoint above to authorize an OAuth grant.</p></div>`)
	}
	endpoint := escape(strings.TrimRight(s.cfg.PublicURL, "/")) + `/mcp`
	body := `<div class="page"><header class="page-header"><div><p class="eyebrow">INTEGRATIONS</p><h1>MCP</h1></div>` +
		`<div class="page-header-actions"><a class="or-button secondary" href="/docs/mcp">MCP docs</a></div></header>` +
		`<section class="content-section"><p class="page-copy">Connect agents via OAuth with an Owner-signed RC Lock grant. Revoking a connection invalidates its tokens and syncs the lock change to affected Nodes.</p>` +
		`<div class="or-copy-field mcp-endpoint-field" data-copy-value="` + endpoint + `"><code>` + endpoint + `</code><button class="or-copy-button copy-value" type="button" aria-label="Copy MCP endpoint"><span class="or-copy-icon" aria-hidden="true"></span></button></div></section>` +
		`<section class="content-section"><div class="section-heading"><div><h2>Connected clients</h2></div></div><div class="data-list">` + rows.String() + `</div></section>` +
		`<p class="error" data-mcp-page-error role="alert"></p></div>`
	sidebar := s.renderSidebar(user.ID, user.Name, "/integrations/mcp")
	s.writePage(w, http.StatusOK, pages.Authenticated("MCP", sidebar, body,
		[]string{"mcp-page", "copy"}, nil, s.assetsVersion()), "no-store")
}
