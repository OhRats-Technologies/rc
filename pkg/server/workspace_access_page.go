package server

import (
	"net/http"
	"strings"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/server/pages"
	"github.com/OhRats-Technologies/rc/pkg/version"
)

func (s *Server) handleWorkspaceAccessPage(w http.ResponseWriter, req *http.Request) {
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
	role, err := s.auth.WorkspaceRole(user.ID, workspaceID)
	if err != nil || role != "owner" {
		http.NotFound(w, req)
		return
	}
	workspace, err := s.auth.WorkspaceByID(workspaceID)
	if err != nil {
		http.NotFound(w, req)
		return
	}
	members, err := s.auth.WorkspaceMemberDetails(workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workspace access could not be loaded")
		return
	}
	invites, err := s.auth.ListWorkspaceInvites(workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workspace access could not be loaded")
		return
	}
	var rows strings.Builder
	for _, member := range members {
		rows.WriteString(workspaceAccessRow(workspaceID, user.ID, member))
	}
	if len(members) == 0 {
		rows.WriteString(`<p class="empty-state">No workspace members.</p>`)
	}
	inviteRows := ""
	for _, invite := range invites {
		inviteRows += `<div class="setting-row invite-row"><div><strong>` + strings.ToUpper(escape(invite.Role)) +
			` invite</strong><div class="meta">EXPIRES ` + escape(invite.ExpiresAt.UTC().Format("2006-01-02 15:04")) +
			`</div></div><div class="row-actions"><form method="post" action="/workspaces/` +
			escape(workspaceID) + `/invites/` + escape(invite.ID) + `/revoke"><button class="text-button danger-text" type="submit">REVOKE</button></form></div></div>`
	}
	if inviteRows == "" {
		inviteRows = `<p class="empty-state">No pending invitations.</p>`
	}
	body := `<div class="page" data-authority-workspace="` + escape(workspaceID) + `">` +
		`<header class="page-header"><div><p class="eyebrow">` + escape(strings.ToUpper(workspace.Name)) + ` / ACCESS</p><h1>Manage access</h1></div></header>` +
		`<section class="content-section"><div class="section-heading"><div><h2>Invite an operator or viewer</h2></div></div>` +
		`<form method="post" action="/workspaces/` + escape(workspaceID) + `/invites" class="inline-form compact-inline-form">` +
		`<label>Role<select name="role"><option value="operator">Operator</option><option value="viewer">Viewer</option></select></label>` +
		`<button class="or-button" type="submit">CREATE INVITE</button></form><div data-invite-result></div></section>` +
		`<section class="content-section"><div class="section-heading"><div><h2>` + itoa(len(members)) + ` ` +
		plural(len(members), "MEMBER", "MEMBERS") + `</h2></div><button class="text-button" type="button" data-authority-sync="` +
		escape(workspaceID) + `">SYNC DEVICE ACCESS</button></div>` +
		`<p class="meta" data-authority-status="` + escape(workspaceID) + `">RC Lock changes are accepted by Nodes only after Owner authorization.</p>` +
		`<div class="settings-list">` + rows.String() + `</div></section>` +
		`<section class="content-section"><h2>Pending invitations</h2><div class="settings-list">` + inviteRows + `</div></section></div>`
	sidebar := s.renderSidebar(user.ID, user.Name, req.URL.Path)
	s.writePage(w, http.StatusOK, pages.Authenticated("Manage access", sidebar, body,
		[]string{"pages", "authority"}, version.Version), "no-store")
}

func workspaceAccessRow(workspaceID, currentUser string, member authstore.WorkspaceMemberDetail) string {
	suffix := ""
	if member.UserID == currentUser {
		suffix = " (you)"
	}
	remove := ""
	if member.UserID != currentUser {
		remove = `<form method="post" action="/workspaces/` + escape(workspaceID) + `/members/` +
			escape(member.UserID) + `/remove"><button class="text-button danger-text" type="submit">REMOVE</button></form>`
	}
	return `<div class="setting-row access-row"><div><strong>` + escape(member.Name) + suffix +
		`</strong><div class="meta">JOINED ` + escape(member.JoinedAt.UTC().Format("2006-01-02")) + `</div></div>` +
		`<div class="row-actions"><form method="post" action="/workspaces/` + escape(workspaceID) + `/members/` +
		escape(member.UserID) + `/role" class="role-form"><select name="role" aria-label="Role for ` + escape(member.Name) + `">` +
		roleOption("owner", member.Role) + roleOption("operator", member.Role) + roleOption("viewer", member.Role) +
		`</select><button class="text-button" type="submit">SAVE</button></form>` + remove +
		`</div><p class="error row-error"></p></div>`
}

func roleOption(value, selected string) string {
	attribute := ""
	if value == selected {
		attribute = " selected"
	}
	return `<option value="` + value + `"` + attribute + `>` + strings.Title(value) + `</option>`
}

func plural(count int, singular, multiple string) string {
	if count == 1 {
		return singular
	}
	return multiple
}
