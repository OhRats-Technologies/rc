package server

import (
	"net/http"
	"strings"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/server/pages"
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
		rows.WriteString(`<div class="empty-card"><h2>No workspace members</h2></div>`)
	}
	status, err := s.workspaceAuthority(workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workspace access could not be loaded")
		return
	}
	inviteRows := ""
	for _, invite := range invites {
		inviteRows += `<div class="key-row invite-row"><div class="key-row-icon"><span class="ui-icon icon-access" aria-hidden="true"></span></div>` +
			`<div class="key-row-body"><div class="key-row-title"><strong>` + strings.Title(escape(invite.Role)) + ` invite</strong>` +
			`<div class="key-row-dates"><span>Expires ` + escape(invite.ExpiresAt.UTC().Format("Jan 2, 2006 at 15:04 UTC")) + `</span></div></div></div>` +
			`<form method="post" action="/workspaces/` + escape(workspaceID) + `/invites/` + escape(invite.ID) + `/revoke">` +
			`<button class="btn-danger-outline" type="submit">Revoke</button></form></div>`
	}
	if inviteRows == "" {
		inviteRows = `<div class="empty-card"><p class="page-copy">No pending invitations.</p></div>`
	}
	body := `<div class="page" data-authority-workspace="` + escape(workspaceID) + `">` +
		`<header class="page-header"><div><p class="eyebrow">` + escape(strings.ToUpper(workspace.Name)) + `</p><h1>Manage access</h1>` +
		`<p class="page-copy">Control who can observe or execute commands on nodes in this workspace.</p></div>` +
		`<div class="page-header-actions"><button class="or-button" type="button" data-authority-sync="` + escape(workspaceID) + `">Sync device access</button></div></header>` +
		accessSummary(status, workspaceID) +
		`<section class="panel-card"><div class="panel-head"><span class="eyebrow">01 · INVITE</span><h2>Invite an operator or viewer</h2></div>` +
		`<div class="panel-body"><form method="post" action="/workspaces/` + escape(workspaceID) + `/invites" class="inline-form compact-inline-form">` +
		`<label>Role<select name="role"><option value="operator">Operator</option><option value="viewer">Viewer</option></select></label>` +
		`<button class="or-button" type="submit">Create invite</button></form><div data-invite-result></div></div></section>` +
		`<section class="panel-card"><div class="panel-head"><span class="eyebrow">02 · MEMBERS</span><h2>` + itoa(len(members)) + ` ` +
		plural(len(members), "member", "members") + `</h2></div>` +
		`<div class="key-list">` + rows.String() + `</div></section>` +
		`<section class="panel-card"><div class="panel-head"><span class="eyebrow">03 · INVITATIONS</span><h2>Pending invitations</h2></div><div class="key-list">` + inviteRows + `</div></section></div>`
	sidebar := s.renderSidebar(user.ID, user.Name, req.URL.Path)
	s.writePage(w, http.StatusOK, pages.Authenticated("Manage access", sidebar, body,
		[]string{"pages", "authority"}, nil, s.assetsVersion()), "no-store")
}

func workspaceAccessRow(workspaceID, currentUser string, member authstore.WorkspaceMemberDetail) string {
	suffix := ""
	if member.UserID == currentUser {
		suffix = " (you)"
	}
	remove := ""
	if member.UserID != currentUser {
		remove = `<form method="post" action="/workspaces/` + escape(workspaceID) + `/members/` +
			escape(member.UserID) + `/remove"><button class="btn-danger-outline" type="submit">Remove</button></form>`
	}
	return `<div class="key-row access-row"><div class="key-row-icon"><span class="ui-icon icon-access" aria-hidden="true"></span></div>` +
		`<div class="key-row-body"><div class="key-row-title"><strong>` + escape(member.Name) + suffix + `</strong>` +
		`<div class="key-row-dates"><span>Joined ` + escape(member.JoinedAt.UTC().Format("Jan 2, 2006")) + `</span></div></div></div>` +
		`<div class="row-actions"><form method="post" action="/workspaces/` + escape(workspaceID) + `/members/` +
		escape(member.UserID) + `/role" class="role-form"><select name="role" aria-label="Role for ` + escape(member.Name) + `">` +
		roleOption("owner", member.Role) + roleOption("operator", member.Role) + roleOption("viewer", member.Role) +
		`</select><button class="or-button secondary" type="submit">Save</button></form>` + remove +
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

// accessSummary states whether every Node accepts the workspace access, or shows the warning when one does not.
func accessSummary(status authorityStatus, workspaceID string) string {
	if len(status.Pending) > 0 {
		return authorityAlert(status, "owner", false)
	}
	text := "No Nodes in this workspace."
	if count := len(status.Nodes); count > 0 {
		text = "All " + itoa(count) + " " + plural(count, "Node accepts", "Nodes accept") + " the current workspace access."
	}
	return `<p class="meta" data-authority-status="` + escape(workspaceID) + `">` + text + `</p>`
}
