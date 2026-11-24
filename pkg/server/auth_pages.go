package server

import (
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"

	"github.com/OhRats-Technologies/rc/pkg/server/pages"
)

func escape(value string) string { return html.EscapeString(value) }

// metaRow renders metadata as separate items so spacing, not a separator
// glyph, carries the grouping. Items must already be escaped.
func metaRow(tag string, items ...string) string {
	var out strings.Builder
	out.WriteString(`<` + tag + ` class="meta meta-row">`)
	for _, item := range items {
		out.WriteString(`<span>` + item + `</span>`)
	}
	out.WriteString(`</` + tag + `>`)
	return out.String()
}

func (s *Server) renderAuthenticated(w http.ResponseWriter, req *http.Request, title, path, body string, scripts []string) {
	user, _, err := s.currentSession(req)
	if err != nil {
		http.Redirect(w, req, "/login?next="+escape(path), http.StatusSeeOther)
		return
	}
	sidebar := s.renderSidebar(user.ID, user.Name, path)
	document := pages.Authenticated(title, sidebar, body, scripts, nil, s.assetsVersion())
	s.writePage(w, http.StatusOK, document, "no-store")
}

func (s *Server) handleDevicesPage(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		http.Redirect(w, req, "/login?next=%2Fdevices", http.StatusSeeOther)
		return
	}
	devices := s.visibleDevices(user.ID)
	workspaceNames := make(map[string]string)
	wsCount := 0
	if workspaces, err := s.auth.WorkspacesForUser(user.ID); err == nil {
		wsCount = len(workspaces)
		for _, ws := range workspaces {
			workspaceNames[ws.ID] = ws.Name
		}
	}
	onlineCount := 0
	var rows strings.Builder
	if len(devices) == 0 {
		rows.WriteString(`<div class="empty-card"><h2>No devices enrolled yet</h2>` +
			`<p class="page-copy">Install the RC Node on a machine to get persistent terminals, process monitoring, and private access without exposing SSH.</p>` +
			`<div class="empty-card-actions"><a class="or-button" href="/devices/enroll">Enroll your first device</a>` +
			`<a class="or-button secondary" href="/docs">Read quickstart</a></div></div>`)
	} else {
		for _, device := range devices {
			if device.Online {
				onlineCount++
			}
			wsName := workspaceNames[device.WorkspaceID]
			if wsName == "" {
				wsName = "Workspace"
			}
			rows.WriteString(deviceListRow(device, wsName))
		}
	}
	offlineCount := len(devices) - onlineCount
	telemetry := `<section class="telemetry-strip" aria-label="Fleet overview">` +
		`<div class="telemetry-cell"><span class="telemetry-label">NODES</span><strong class="telemetry-value">` + itoa(len(devices)) + `</strong></div>` +
		`<div class="telemetry-cell is-online"><span class="telemetry-label">ONLINE</span><strong class="telemetry-value">` + itoa(onlineCount) + `</strong></div>` +
		`<div class="telemetry-cell"><span class="telemetry-label">OFFLINE</span><strong class="telemetry-value">` + itoa(offlineCount) + `</strong></div>` +
		`<div class="telemetry-cell"><span class="telemetry-label">WORKSPACES</span><strong class="telemetry-value">` + itoa(wsCount) + `</strong></div>` +
		`</section>`
	listHeader := ""
	if len(devices) > 0 {
		listHeader = `<div class="fleet-deck-head"><span>NODE</span><span class="fleet-col-host">HOST</span><span class="fleet-col-platform">PLATFORM</span><span class="fleet-col-status">STATUS</span></div>`
	}
	body := `<div class="page" data-live-page="devices"><header class="page-header"><div><p class="eyebrow">CONTROL PLANE</p><h1>Devices</h1></div>` +
		`<div class="page-header-actions"><a class="or-button secondary" href="/docs/cli">CLI docs</a><a class="or-button" href="/devices/enroll">Enroll device</a></div></header>` +
		telemetry +
		`<section class="fleet-deck">` + listHeader + `<div class="data-list" id="device-list">` + rows.String() + `</div></section></div>`
	sidebar := s.renderSidebar(user.ID, user.Name, "/devices")
	s.writePage(w, http.StatusOK, pages.Authenticated("Devices", sidebar, body, []string{"live"}, nil, s.assetsVersion()), "no-store")
}

func formatPlatform(platform, arch string) string {
	osName := strings.TrimSpace(platform)
	switch strings.ToLower(osName) {
	case "darwin", "macos":
		osName = "macOS"
	case "linux":
		osName = "Linux"
	case "windows":
		osName = "Windows"
	}
	if arch == "" {
		return osName
	}
	return osName + " · " + arch
}

func deviceListRow(device *RegisteredDevice, workspaceName string) string {
	status := "OFFLINE"
	if device.Online {
		status = "ONLINE"
	}
	return `<a class="data-row fleet-row" href="/devices/` + escape(device.ID) + `" data-device-row="` + escape(device.ID) + `">` +
		`<div class="device-row-main"><span class="device-row-badge"><span class="ui-icon device-platform-icon ` + platformIcon(device.Platform) + `"></span></span>` +
		`<div class="device-row-text"><strong>` + escape(device.Name) + `</strong><span class="device-row-ws">` + escape(workspaceName) + `</span></div></div>` +
		`<span class="fleet-col-host fleet-host">` + escape(device.Hostname) + `</span>` +
		`<span class="fleet-col-platform"><span class="cap-chip"><span>` + escape(formatPlatform(device.Platform, device.Arch)) + `</span></span></span>` +
		`<span class="fleet-col-status fleet-status-wrap"><span class="status` + onlineClass(device.Online) + `" data-device-status="` + escape(device.ID) + `">` + status + `</span><span class="fleet-arrow" aria-hidden="true">→</span></span></a>`
}

func platformIcon(platform string) string {
	switch strings.ToLower(platform) {
	case "darwin", "macos":
		return "icon-platform-macos"
	case "windows":
		return "icon-platform-windows"
	default:
		return "icon-platform-linux"
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
	role, err := s.auth.WorkspaceRole(user.ID, device.WorkspaceID)
	if err != nil {
		http.NotFound(w, req)
		return
	}
	access, err := s.deviceAuthority(device)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "device could not be loaded")
		return
	}
	status := "OFFLINE"
	if device.Online {
		status = "ONLINE"
	}
	supportsProcess := false
	for _, capability := range device.Capabilities {
		if capability == "process" || capability == "pty" || capability == "shell" {
			supportsProcess = true
		}
	}
	terminalDisabled := "disabled"
	terminalTitle := "Device is offline"
	if device.Online && supportsProcess {
		terminalDisabled = ""
		terminalTitle = "Open interactive terminal"
	} else if device.Online && !supportsProcess {
		terminalTitle = "This node does not expose terminal control"
	}
	workspaceName := "Workspace"
	if ws, err := s.auth.WorkspaceByID(device.WorkspaceID); err == nil && ws != nil && strings.TrimSpace(ws.Name) != "" {
		workspaceName = ws.Name
	}
	body := `<div class="page" data-device-page="` + escape(device.ID) + `" data-supports-process="` + fmt.Sprint(supportsProcess) + `" data-retains-process-history="false">` +
		`<section class="device-overview">` +
		`<header class="page-header device-header"><div class="device-identity"><span class="device-badge"><span class="ui-icon device-platform-icon device-title-platform ` + platformIcon(device.Platform) + `"></span></span><div class="device-identity-copy">` +
		`<div class="page-title-row device-title-row"><h1 data-device-title-view>` + escape(device.Name) + `</h1>` +
		`<form method="post" action="/devices/` + escape(device.ID) + `/rename" hidden data-device-title-form><input class="device-title-input" name="name" value="` + escape(device.Name) + `" aria-label="Device name" required maxlength="120"><input type="hidden" name="next" value="/devices/` + escape(device.ID) + `"></form>` +
		`<button class="header-icon-button" type="button" data-device-title-rename aria-label="Rename device" title="Rename device"><span class="ui-icon icon-pencil"></span></button></div>` +
		`<p class="device-subtitle"><span>` + escape(workspaceName) + `</span><span class="dot-sep" aria-hidden="true">·</span><span>` + escape(device.Hostname) + `</span></p>` +
		`<p class="error device-title-error" data-device-title-error></p></div></div>` +
		`<div class="device-header-actions"><span id="device-status" class="status` + onlineClass(device.Online) + `">` + status + `</span>` +
		`<button id="open-terminal" class="or-button device-terminal-button" type="button" title="` + escape(terminalTitle) + `" ` + terminalDisabled + `><span class="ui-icon icon-terminal" aria-hidden="true"></span><span>Open terminal</span></button>` +
		`<span class="action-divider" aria-hidden="true"></span>` +
		`<button class="header-icon-button danger-icon-button" type="button" aria-label="Delete ` + escape(device.Name) + `" title="Delete device" data-delete-kind="device" data-delete-name="` + escape(device.Name) + `" data-delete-description="Deletes from RC immediately. If the Node is offline, it clears its old enrollment the next time it connects." data-delete-endpoint="/api/v1/devices/` + escape(device.ID) + `" data-access-workspace="` + escape(device.WorkspaceID) + `" data-delete-redirect="/devices"><span class="ui-icon icon-trash"></span></button></div></header>` +
		`<dl class="facts"><div><dt>HOSTNAME</dt><dd>` + escape(device.Hostname) + `</dd></div><div><dt>PLATFORM</dt><dd>` + escape(strings.ToUpper(device.Platform)) + ` / ` + escape(device.Arch) + `</dd></div><div><dt>NODE VERSION</dt><dd id="node-version">` + escape(device.Version) + `</dd></div><div><dt>TRANSPORT</dt><dd>End-to-end · RC Lock</dd></div></dl>` +
		`<p id="process-error" class="error"></p></section>` +
		deviceAuthorityAlert(access, role) +
		`<div class="device-workspace">` +
		`<section class="panel-card"><div class="panel-head"><span class="eyebrow">01 · SESSIONS</span><h2>Active processes</h2></div>` +
		`<div class="data-list" id="process-list"><div class="console-empty"><div class="console-preview"><span class="console-prompt">$</span> <span>rc shell ` + escape(device.Name) + `</span></div><p class="page-copy">No active processes running on this node. Launch an interactive terminal above or run a command from the CLI.</p></div></div></section>` +
		`<aside class="panel-card quick-cli-card"><div class="panel-head"><span class="eyebrow">02 · CLI ACCESS</span><h2>Quick commands</h2></div>` +
		`<div class="quick-cli-list"><div class="quick-cli-item"><span class="meta">INTERACTIVE SHELL</span><div class="or-copy-field" data-copy-value="rc shell ` + escape(device.Name) + `"><code>rc shell ` + escape(device.Name) + `</code><button class="or-copy-button copy-value" type="button" aria-label="Copy shell command"><span class="or-copy-icon" aria-hidden="true"></span></button></div></div>` +
		`<div class="quick-cli-item"><span class="meta">RUN ONE-OFF COMMAND</span><div class="or-copy-field" data-copy-value="rc shell ` + escape(device.Name) + ` --command &quot;uname -a&quot;"><code>rc shell ` + escape(device.Name) + ` --command &quot;uname -a&quot;</code><button class="or-copy-button copy-value" type="button" aria-label="Copy run command"><span class="or-copy-icon" aria-hidden="true"></span></button></div></div>` +
		`<div class="quick-cli-item"><span class="meta">MCP &amp; API</span><p class="meta">Use <a class="inline-link" href="/integrations/mcp">MCP</a> or <a class="inline-link" href="/api">API keys</a> to give agents scoped access to this node.</p></div></div></aside>` +
		`</div></div>`
	sidebar := s.renderSidebar(user.ID, user.Name, req.URL.Path)
	s.writePage(w, http.StatusOK, pages.Authenticated(device.Name, sidebar, body, []string{"live", "device", "authority", "copy"}, nil, s.assetsVersion()), "no-store")
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
	workspaces, _ := s.auth.WorkspacesForUser(user.ID)
	selected := strings.TrimSpace(req.URL.Query().Get("workspace"))
	var options strings.Builder
	for _, workspace := range workspaces {
		if workspace.Role != "owner" {
			continue
		}
		if selected == "" {
			selected = workspace.ID
		}
	}
	for _, workspace := range workspaces {
		if workspace.Role != "owner" {
			continue
		}
		attribute := ""
		if workspace.ID == selected {
			attribute = " selected"
		}
		options.WriteString(`<option value="` + escape(workspace.ID) + `"` + attribute + `>` + escape(workspace.Name) + `</option>`)
	}
	if options.Len() == 0 {
		options.WriteString(`<option value="">No owned workspaces</option>`)
	}
	body := `<div class="page narrow-form-page" data-enroll-page="` + escape(selected) + `" data-known-devices="` + escape(strings.Join(ids, ",")) + `">` +
		`<header class="page-header"><div><p class="eyebrow">DEVICES</p><h1>Enroll a device</h1>` +
		`<p class="page-copy">Generate a one-time install command to connect a new machine to your workspace.</p></div></header>` +
		`<section class="panel-card"><div class="panel-head"><span class="eyebrow">STEP 01</span><h2>Choose workspace &amp; platform</h2></div>` +
		`<div class="panel-body"><form data-enrollment-form class="simple-form"><div class="form-grid-2"><label>Workspace<select name="workspaceId">` + options.String() + `</select></label>` +
		`<label>Target platform<select name="platform"><option value="unix">Linux / macOS</option><option value="windows">Windows (PowerShell)</option><option value="android">Android (Termux)</option></select></label></div>` +
		`<p class="page-copy" data-enrollment-platform-help>Run the generated command in a terminal on the machine you want to enroll.</p>` +
		`<button class="or-button" type="submit">Generate enrollment command</button></form></div></section>` +
		`<div class="enrollment-command">` +
		`<div class="panel-card enrollment-block"><div class="panel-head"><span class="eyebrow">NEW MACHINE · SHOWN ONCE</span><h2>Install &amp; enroll</h2></div><div class="panel-body"><div class="or-copy-field enrollment-install" data-enrollment-copy-field hidden><code data-enrollment-result="install"></code><button class="or-copy-button" type="button" aria-label="Copy install command" data-enrollment-copy><span class="or-copy-icon" aria-hidden="true"></span></button></div></div></div>` +
		`<div class="panel-card enrollment-block"><div class="panel-head"><span class="eyebrow">ALREADY INSTALLED · SHOWN ONCE</span><h2>Enroll existing RC Node</h2></div><div class="panel-body"><div class="or-copy-field enrollment-install" data-enrollment-copy-field hidden><code data-enrollment-result="enroll"></code><button class="or-copy-button" type="button" aria-label="Copy enroll command" data-enrollment-copy><span class="or-copy-icon" aria-hidden="true"></span></button></div></div></div>` +
		`<p class="page-copy">One default background enrollment is supported per OS user. RC refuses to overwrite an existing device identity.</p>` +
		`<p class="error" role="alert" data-enrollment-error></p><p id="enrollment-state" class="meta"></p></div></div>`
	sidebar := s.renderSidebar(user.ID, user.Name, "/devices/enroll")
	s.writePage(w, http.StatusOK, pages.Authenticated("Enroll device", sidebar, body, []string{"pages", "live"}, nil, s.assetsVersion()), "no-store")
}
