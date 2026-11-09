package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
	"github.com/OhRats-Technologies/rc/pkg/server/pages"
	"github.com/OhRats-Technologies/rc/pkg/version"
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
			`</strong><div class="meta">` + escape(grant.ClientID) + ` · ` +
			escape(strings.Join(payload.Scopes, " ")) + ` · ` + escape(expiry) +
			`</div></div><button class="or-button secondary" type="button" data-mcp-revoke="` +
			escape(grant.ID) + `">Revoke</button></div>`)
	}
	if rows.Len() == 0 {
		rows.WriteString(`<p class="empty-state">No MCP clients are connected.</p>`)
	}
	body := `<div class="page"><header class="page-header"><div><h1>MCP</h1></div></header>` +
		`<section class="content-section"><p class="page-copy">Connect agents to <code>` +
		escape(strings.TrimRight(s.cfg.PublicURL, "/")) + `/mcp</code>. Each connection uses OAuth with ` +
		`an Owner-signed RC Lock grant. Revoking a connection invalidates its tokens and synchronizes ` +
		`the lock change to affected Nodes.</p></section>` +
		`<div class="data-list">` + rows.String() + `</div>` +
		`<p class="error" data-mcp-page-error role="alert"></p></div>`
	sidebar := s.renderSidebar(user.ID, user.Name, "/integrations/mcp")
	s.writePage(w, http.StatusOK, pages.Authenticated("MCP", sidebar, body,
		[]string{"mcp-page"}, version.Version), "no-store")
}
