package server

import (
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"

	"github.com/OhRats-Technologies/rc/pkg/server/pages"
	"github.com/OhRats-Technologies/rc/pkg/version"
)

func escape(value string) string { return html.EscapeString(value) }

func (s *Server) renderAuthenticated(w http.ResponseWriter, req *http.Request, title, path, body string, scripts []string) {
	user, _, err := s.currentSession(req)
	if err != nil {
		http.Redirect(w, req, "/login?next="+escape(path), http.StatusSeeOther)
		return
	}
	sidebar := s.renderSidebar(user.ID, user.Name, path)
	document := pages.Authenticated(title, sidebar, body, scripts, version.Version)
	s.writePage(w, http.StatusOK, document, "no-store")
}

func (s *Server) handleDevicesPage(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		http.Redirect(w, req, "/login?next=%2Fdevices", http.StatusSeeOther)
		return
	}
	devices := s.visibleDevices(user.ID)
	var rows strings.Builder
	if len(devices) == 0 {
		rows.WriteString(`<p class="empty-state">No devices yet. Enroll one from a workspace.</p>`)
	} else {
		for _, device := range devices {
			rows.WriteString(deviceListRow(device))
		}
	}
	body := `<div class="page" data-live-page="devices"><header class="page-header"><div><h1>Devices</h1></div>` +
		`<a class="header-icon-button" href="/devices/enroll" aria-label="Enroll device" title="Enroll device"><span class="ui-icon icon-plus"></span></a></header>` +
		`<div class="data-list" id="device-list">` + rows.String() + `</div></div>`
	sidebar := s.renderSidebar(user.ID, user.Name, "/devices")
	s.writePage(w, http.StatusOK, pages.Authenticated("Devices", sidebar, body, []string{"live"}, version.Version), "no-store")
}

func deviceListRow(device *RegisteredDevice) string {
	status := "OFFLINE"
	if device.Online {
		status = "ONLINE"
	}
	return `<a class="data-row" href="/devices/` + escape(device.ID) + `" data-device-row="` + escape(device.ID) + `">` +
		`<div class="device-row-main"><span class="ui-icon device-platform-icon ` + platformIcon(device.Platform) + `"></span><div><strong>` + escape(device.Name) + `</strong>` +
		`<div class="meta">Personal · ` + escape(strings.ToUpper(device.Platform)) + `/` + escape(device.Arch) + `</div></div></div>` +
		`<span class="status` + onlineClass(device.Online) + `" data-device-status="` + escape(device.ID) + `">` + status + `</span></a>`
}

func platformIcon(platform string) string {
	switch strings.ToLower(platform) {
	case "darwin", "macos":
		return "platform-macos"
	case "windows":
		return "platform-windows"
	default:
		return "platform-linux"
	}
}

func onlineClass(online bool) string {
	if online {
		return " online"
	}
	return ""
}

func (s *Server) handleDevicePage(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		http.Redirect(w, req, "/login?next="+escape(req.URL.Path), http.StatusSeeOther)
		return
	}
	device, ok := s.registry.Get(req.PathValue("id"))
	if !ok {
		http.NotFound(w, req)
		return
	}
	if _, err := s.auth.WorkspaceRole(user.ID, device.WorkspaceID); err != nil {
		http.NotFound(w, req)
		return
	}
	status := "OFFLINE"
	if device.Online {
		status = "ONLINE"
	}
	supportsProcess := false
	for _, capability := range device.Capabilities {
		if capability == "process" {
			supportsProcess = true
		}
	}
	terminalDisabled := "disabled"
	if device.Online && supportsProcess {
		terminalDisabled = ""
	}
	body := `<div class="page" data-device-page="` + escape(device.ID) + `" data-supports-process="` + fmt.Sprint(supportsProcess) + `" data-retains-process-history="false">` +
		`<section class="device-overview"><header class="page-header device-header"><div><p class="eyebrow">DEVICE</p>` +
		`<div class="page-title-row device-title-row"><span class="ui-icon device-platform-icon device-title-platform ` + platformIcon(device.Platform) + `"></span>` +
		`<h1 data-device-title-view>` + escape(device.Name) + `</h1>` +
		`<form method="post" action="/devices/` + escape(device.ID) + `/rename" hidden data-device-title-form><input class="device-title-input" name="name" value="` + escape(device.Name) + `" aria-label="Device name" required maxlength="120"><input type="hidden" name="next" value="/devices/` + escape(device.ID) + `"></form>` +
		`<button class="header-icon-button" type="button" data-device-title-rename aria-label="Rename device" title="Rename device"><span class="ui-icon icon-pencil"></span></button></div>` +
		`<p class="error device-title-error" data-device-title-error></p><p class="meta">Personal · ` + escape(strings.ToUpper(device.Platform)) + `/` + escape(device.Arch) + `</p></div>` +
		`<div class="device-header-actions"><span id="device-status" class="status` + onlineClass(device.Online) + `">` + status + `</span>` +
		`<button id="open-terminal" class="device-terminal-button" type="button" aria-label="Open terminal" title="Terminal control is not available in this build" ` + terminalDisabled + `><span class="ui-icon icon-terminal"></span></button>` +
		`<button class="header-icon-button danger-icon-button" type="button" aria-label="Delete ` + escape(device.Name) + `" title="Delete device" data-delete-kind="device" data-delete-name="` + escape(device.Name) + `" data-delete-description="Deletes from RC immediately. If the Node is offline, it clears its old enrollment the next time it connects." data-delete-endpoint="/api/v1/devices/` + escape(device.ID) + `" data-delete-redirect="/devices"><span class="ui-icon icon-trash"></span></button></div></header>` +
		`<dl class="facts"><div><dt>HOST</dt><dd>` + escape(device.Hostname) + `</dd></div><div><dt>NODE VERSION</dt><dd id="node-version">` + escape(device.Version) + `</dd></div></dl>` +
		`<p id="process-error" class="error"></p></section>` +
		`<section class="content-section"><div class="section-heading"><div><div class="badge-container"><span class="badge-text">01 Processes</span><div class="badge-line"></div></div><h2>Active</h2></div></div>` +
		`<div class="data-list" id="process-list"><p class="empty-state">No active processes.</p></div></section></div>`
	sidebar := s.renderSidebar(user.ID, user.Name, req.URL.Path)
	s.writePage(w, http.StatusOK, pages.Authenticated(device.Name, sidebar, body, []string{"live", "device"}, version.Version), "no-store")
}

func (s *Server) handleEnrollPage(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		http.Redirect(w, req, "/login?next=%2Fdevices%2Fenroll", http.StatusSeeOther)
		return
	}
	devices := s.visibleDevices(user.ID)
	ids := make([]string, 0, len(devices))
	for _, device := range devices {
		ids = append(ids, device.ID)
	}
	sort.Strings(ids)
	body := `<div class="page narrow-form-page" data-enroll-page="ws_default" data-known-devices="` + escape(strings.Join(ids, ",")) + `">` +
		`<header class="page-header"><div><p class="eyebrow">DEVICES</p><h1>Enroll device</h1></div></header>` +
		`<form data-enrollment-form class="simple-form"><label>Workspace<select name="workspaceId"><option value="ws_default" selected>Personal</option></select></label>` +
		`<label>Target platform<select name="platform"><option value="unix">Linux / macOS</option><option value="windows">Windows (PowerShell)</option></select></label>` +
		`<p class="meta" data-enrollment-platform-help>Run the command in a terminal on the machine you want to enroll.</p>` +
		`<button class="or-button" type="submit">CREATE ENROLLMENT COMMANDS</button></form>` +
		`<div class="enrollment-command"><span class="meta">NEW INSTALL · SHOWN ONCE</span><div class="or-copy-field enrollment-install" data-enrollment-copy-field hidden><code data-enrollment-result="install"></code><button class="or-copy-button" type="button" aria-label="Copy install command" data-enrollment-copy><span class="or-copy-icon" aria-hidden="true"></span></button></div>` +
		`<span class="meta">ALREADY INSTALLED · SHOWN ONCE</span><div class="or-copy-field enrollment-install" data-enrollment-copy-field hidden><code data-enrollment-result="enroll"></code><button class="or-copy-button" type="button" aria-label="Copy enroll command" data-enrollment-copy><span class="or-copy-icon" aria-hidden="true"></span></button></div>` +
		`<p class="meta">One default background enrollment is supported per OS user. RC refuses to overwrite an existing device identity.</p>` +
		`<p class="error" role="alert" data-enrollment-error></p><p id="enrollment-state" class="meta"></p></div></div>`
	sidebar := s.renderSidebar(user.ID, user.Name, "/devices/enroll")
	s.writePage(w, http.StatusOK, pages.Authenticated("Enroll device", sidebar, body, []string{"pages", "live"}, version.Version), "no-store")
}

func (s *Server) handleAccountPage(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		http.Redirect(w, req, "/login?next=%2Faccount", http.StatusSeeOther)
		return
	}
	passkeys, _ := s.auth.PasskeysForUser(user.ID)
	var rows strings.Builder
	if len(passkeys) == 0 {
		rows.WriteString(`<p class="empty-state">No passkeys registered.</p>`)
	} else {
		for index, passkey := range passkeys {
			rows.WriteString(`<div class="setting-row"><div><strong>Passkey ` + fmt.Sprint(index+1) + `</strong><div class="meta">` + escape(passkey.Name) + `</div></div>` +
				`<span class="meta">` + strings.ToUpper(passkey.CreatedAt.Format("2006-01-02")) + `</span></div>`)
		}
	}
	body := `<div class="page"><header class="page-header account-header"><div><div class="page-title-row"><h1 data-account-name-view>` + escape(user.Name) + `</h1>` +
		`<form method="post" action="/account/name" hidden data-account-name-form><input class="account-title-input" name="name" value="` + escape(user.Name) + `" aria-label="Account name" required maxlength="120"></form>` +
		`<button class="header-icon-button" type="button" data-account-rename aria-label="Rename account" title="Rename account"><span class="ui-icon icon-pencil"></span></button></div>` +
		`<p class="error account-title-error"></p></div></header><section class="content-section"><div class="section-heading"><div><div class="badge-container"><span class="badge-text">01 Passkeys</span><div class="badge-line"></div></div><h2>Sign-in credentials</h2></div></div>` +
		`<div class="settings-list">` + rows.String() + `</div><p id="passkey-error" class="error"></p></section></div>`
	sidebar := s.renderSidebar(user.ID, user.Name, "/account")
	s.writePage(w, http.StatusOK, pages.Authenticated("Account", sidebar, body, []string{"account"}, version.Version), "no-store")
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
