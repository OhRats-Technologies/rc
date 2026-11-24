package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/OhRats-Technologies/rc/pkg/server/pages"
)

func (s *Server) handleAccountPage(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		http.Redirect(w, req, "/login?next=%2Faccount", http.StatusSeeOther)
		return
	}
	passkeys, _ := s.auth.PasskeysForUser(user.ID)
	initial := "?"
	if trimmed := strings.TrimSpace(user.Name); trimmed != "" {
		initial = strings.ToUpper(string([]rune(trimmed)[0]))
	}
	var rows strings.Builder
	if len(passkeys) == 0 {
		rows.WriteString(`<div class="empty-card"><h2>No passkeys registered</h2><p class="page-copy">Add a hardware-backed passkey to sign in securely without passwords.</p></div>`)
	} else {
		for index, passkey := range passkeys {
			label := strings.TrimSpace(passkey.Name)
			if label == "" {
				label = "WebAuthn passkey"
			}
			rows.WriteString(`<div class="key-row"><div class="key-row-icon"><span class="ui-icon icon-key" aria-hidden="true"></span></div>` +
				`<div class="key-row-body"><div class="key-row-title"><strong>Passkey ` + fmt.Sprint(index+1) + `</strong>` +
				`<div class="key-row-dates"><span>` + escape(label) + `</span><span class="dot-sep" aria-hidden="true">·</span><span>Added ` + escape(passkey.CreatedAt.UTC().Format("Jan 2, 2006")) + `</span></div></div></div>` +
				`<span class="cap-chip"><span>WebAuthn</span></span></div>`)
		}
	}
	body := `<div class="page"><header class="page-header account-header"><div class="device-identity"><span class="account-avatar">` + escape(initial) + `</span><div class="device-identity-copy">` +
		`<p class="eyebrow">ACCOUNT</p><div class="page-title-row"><h1 data-account-name-view>` + escape(user.Name) + `</h1>` +
		`<form method="post" action="/account/name" hidden data-account-name-form><input class="account-title-input" name="name" value="` + escape(user.Name) + `" aria-label="Account name" required maxlength="120"></form>` +
		`<button class="header-icon-button" type="button" data-account-rename aria-label="Rename account" title="Rename account"><span class="ui-icon icon-pencil"></span></button></div>` +
		`<p class="error account-title-error"></p></div></div></header>` +
		`<section class="panel-card"><div class="panel-head"><span class="eyebrow">01 · CREDENTIALS</span><h2>Passkeys (` + itoa(len(passkeys)) + `)</h2></div>` +
		`<div class="key-list">` + rows.String() + `</div><p id="passkey-error" class="error"></p></section>` +
		`<section class="panel-card danger-card"><div class="panel-head"><span class="eyebrow danger-eyebrow">02 · DANGER ZONE</span><h2>Delete account</h2></div>` +
		`<div class="panel-body danger-body"><p class="page-copy">Permanently deletes this account, every workspace you own, and all workspace memberships. Workspaces owned by others remain intact.</p>` +
		`<button class="btn-danger-solid" type="button" data-delete-kind="account" data-delete-name="` + escape(user.Name) +
		`" data-delete-endpoint="/account/delete" data-delete-method="POST" data-delete-redirect="/" data-delete-description="This cannot be undone.">Delete account</button></div></section></div>`
	sidebar := s.renderSidebar(user.ID, user.Name, "/account")
	s.writePage(w, http.StatusOK, pages.Authenticated("Account", sidebar, body, []string{"account"}, nil, s.assetsVersion()), "no-store")
}

func (s *Server) handleAccountName(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	name := strings.TrimSpace(req.FormValue("name"))
	if name == "" {
		var input struct {
			Name string `json:"name"`
		}
		if err := decodeJSON(req, &input); err == nil {
			name = strings.TrimSpace(input.Name)
		}
	}
	updated, err := s.auth.UpdateUserName(user.ID, name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.Contains(req.Header.Get("Accept"), "application/json") {
		writeJSON(w, http.StatusOK, map[string]any{"name": updated.Name})
		return
	}
	http.Redirect(w, req, "/account", http.StatusSeeOther)
}

func (s *Server) handleDeviceRenameForm(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	device, ok := s.registry.Get(req.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	role, err := s.auth.WorkspaceRole(user.ID, device.WorkspaceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	if role != "owner" && role != "operator" {
		writeError(w, http.StatusForbidden, "operator access required")
		return
	}
	renamed, err := s.registry.Rename(req.PathValue("id"), req.FormValue("name"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	next := req.FormValue("next")
	if !safeNext(next) {
		next = "/devices/" + renamed.ID
	}
	http.Redirect(w, req, next, http.StatusSeeOther)
}
