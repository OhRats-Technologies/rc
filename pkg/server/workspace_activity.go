package server

import (
	"encoding/json"
	"net/http"
	"strings"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/server/pages"
)

func (s *Server) handleWorkspaceActivityAPI(w http.ResponseWriter, req *http.Request) {
	userID, ok := requestUserID(req)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	workspaceID := trimWorkspaceID(req.PathValue("id"))
	if workspaceID == "" {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	if _, err := s.auth.WorkspaceRole(userID, workspaceID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	events, err := s.auth.WorkspaceAuditEvents(workspaceID, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "activity could not be loaded")
		return
	}
	rows := make([]map[string]any, 0, len(events))
	for _, event := range events {
		var detail map[string]any
		_ = json.Unmarshal([]byte(event.Detail), &detail)
		rows = append(rows, map[string]any{
			"id": event.ID, "kind": event.Kind, "detail": detail,
			"deviceId": event.DeviceID, "createdAt": event.CreatedAt.UnixMilli(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": rows})
}

func (s *Server) handleWorkspaceActivityPage(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		http.Redirect(w, req, "/login?next="+escape(req.URL.Path), http.StatusSeeOther)
		return
	}
	workspaceID := trimWorkspaceID(req.PathValue("id"))
	if workspaceID == "" {
		http.NotFound(w, req)
		return
	}
	if _, err := s.auth.WorkspaceRole(user.ID, workspaceID); err != nil {
		http.NotFound(w, req)
		return
	}
	workspace, err := s.auth.WorkspaceByID(workspaceID)
	if err != nil {
		http.NotFound(w, req)
		return
	}
	events, err := s.auth.WorkspaceAuditEvents(workspaceID, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "activity could not be loaded")
		return
	}
	var rows strings.Builder
	for _, event := range events {
		rows.WriteString(activityRow(event))
	}
	if rows.Len() == 0 {
		rows.WriteString(`<div class="empty-card"><p class="page-copy">No workspace activity recorded yet.</p></div>`)
	}
	body := `<div class="page" data-activity-page="` + escape(workspaceID) + `">` +
		`<header class="page-header"><div><p class="eyebrow">` + escape(strings.ToUpper(workspace.Name)) + `</p><h1>Activity</h1>` +
		`<p class="page-copy">Audit log of device enrollments, access changes, and sessions.</p></div></header>` +
		`<section class="panel-card"><div class="panel-head"><span class="eyebrow">AUDIT LOG</span><h2>Recent events</h2></div><div id="activity-list" class="activity-list">` + rows.String() + `</div></section></div>`
	sidebar := s.renderSidebar(user.ID, user.Name, req.URL.Path)
	s.writePage(w, http.StatusOK, pages.Authenticated("Activity", sidebar, body,
		[]string{"live"}, nil, s.assetsVersion()), "no-store")
}

func activityRow(event authstore.AuditEvent) string {
	var detail map[string]any
	_ = json.Unmarshal([]byte(event.Detail), &detail)
	value := ""
	for _, key := range []string{"name", "deviceId", "memberId", "inviteId"} {
		if text, ok := detail[key].(string); ok && text != "" {
			value = text
			break
		}
	}
	return `<div class="activity-row"><span class="cap-chip"><span>` + escape(strings.ReplaceAll(event.Kind, ".", " · ")) + `</span></span><span class="activity-target">` +
		escape(value) + `</span><time>` + escape(event.CreatedAt.UTC().Format("Jan 2, 2006 · 15:04")) + `</time></div>`
}
