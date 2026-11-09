package server

import "strings"

func (s *Server) renderSidebar(userID, userName, path string) string {
	current := ""
	if rest, ok := strings.CutPrefix(path, "/devices/"); ok {
		current = strings.SplitN(rest, "/", 2)[0]
	}
	devices := s.visibleDevices(userID)
	var rows strings.Builder
	for _, device := range devices {
		rows.WriteString(s.sidebarDeviceRow(device, current))
	}
	children := `<span class="workspace-empty">No devices</span>`
	if rows.Len() > 0 {
		children = rows.String()
	}
	open := strings.HasPrefix(path, "/devices")
	hidden := ""
	if !open {
		hidden = "hidden"
	}
	initial := "?"
	if trimmed := strings.TrimSpace(userName); trimmed != "" {
		initial = strings.ToUpper(string([]rune(trimmed)[0]))
	}
	return `<aside id="site-sidebar" class="site-sidebar"><div class="sidebar-scroll">` +
		`<a class="site-brand" href="/devices"><img src="https://assets.ohrats.party/assets/logo.092a1cece4d0.svg" alt=""><strong>RC</strong></a>` +
		`<nav aria-label="RC navigation"><section class="sidebar-section"><h2>Navigation</h2>` +
		`<a class="nav-link` + activeClass(path, "/devices") + `" href="/devices"><span class="ui-icon icon-devices"></span><span>Devices</span></a>` +
		`<a class="nav-link` + activeClass(path, "/api") + `" href="/api"><span class="ui-icon icon-key"></span><span>API</span></a>` +
		`<a class="nav-link` + activeClass(path, "/integrations/mcp") + `" href="/integrations/mcp"><span class="ui-icon icon-api"></span><span>MCP</span></a>` +
		`</section><section class="sidebar-section workspace-section"><div class="sidebar-section-title"><h2>Workspaces</h2></div>` +
		`<div class="workspace-folder active" data-workspace-folder="ws_default" data-default-open="true">` +
		`<div class="workspace-folder-head"><button class="workspace-toggle" type="button" aria-expanded="true" data-workspace-toggle="ws_default"><span class="ui-icon icon-folder"></span><span class="workspace-name">Personal</span></button></div>` +
		`<div class="workspace-children" data-workspace-children="ws_default" data-open="true" ` + hidden + `>` + children + `</div></div>` +
		`</section></nav></div><div class="sidebar-footer"><div class="profile-row">` +
		`<a class="profile-link" href="/account"><span class="profile-initial">` + escape(initial) + `</span><span class="profile-name">` + escape(userName) + `</span></a>` +
		`<button class="theme-toggle" type="button" data-theme-toggle aria-label="Toggle theme"></button>` +
		`<form method="post" action="/account/logout"><button class="icon-button" type="submit" aria-label="Sign out" title="Sign out"><span class="ui-icon icon-sign-out"></span></button></form>` +
		`</div></div></aside><button id="sidebar-toggle" class="sidebar-toggle" type="button" aria-label="Toggle sidebar"><span class="ui-icon icon-sidebar"></span></button>` +
		deleteDialog()
}

func activeClass(path, target string) string {
	if path == target || strings.HasPrefix(path, target+"/") {
		return " active"
	}
	return ""
}

func (s *Server) sidebarDeviceRow(device *RegisteredDevice, current string) string {
	active := ""
	if device.ID == current {
		active = " active"
	}
	online := " online"
	if !device.Online {
		online = ""
	}
	status := "OFFLINE"
	if device.Online {
		status = "ONLINE"
	}
	_ = status
	return `<div class="workspace-device-row` + active + `" data-sidebar-device="` + escape(device.ID) + `">` +
		`<div class="workspace-device-head has-menu"><a class="workspace-device-link" href="/devices/` + escape(device.ID) + `" data-device-name-view>` +
		`<span class="workspace-device-presence` + online + `" data-sidebar-device-status="` + escape(device.ID) + `"></span>` +
		`<span class="workspace-device-name"><span>` + escape(device.Name) + `</span></span></a>` +
		`<details class="workspace-menu device-menu"><summary class="workspace-menu-trigger" aria-label="Actions for ` + escape(device.Name) + `"><span class="ui-icon icon-ellipsis"></span></summary>` +
		`<div class="workspace-menu-popover"><div class="workspace-menu-actions">` +
		`<form class="device-inline-rename" method="post" action="/devices/` + escape(device.ID) + `/rename" hidden data-device-rename-form>` +
		`<input class="device-rename-input" name="name" value="` + escape(device.Name) + `" aria-label="Rename ` + escape(device.Name) + `" required maxlength="120">` +
		`<input type="hidden" name="next" value="/devices/` + escape(device.ID) + `"></form>` +
		`<button type="button" data-device-rename><span class="ui-icon icon-pencil"></span>Rename device</button>` +
		`<button class="danger-text" type="button" data-delete-kind="device" data-delete-name="` + escape(device.Name) + `" data-delete-endpoint="/api/v1/devices/` + escape(device.ID) + `"><span class="ui-icon icon-trash"></span>Delete device</button>` +
		`</div></div></details></div></div>`
}

func deleteDialog() string {
	return `<dialog class="delete-dialog" data-delete-dialog aria-labelledby="delete-dialog-title"><div class="delete-dialog-content">` +
		`<h2 id="delete-dialog-title" data-delete-title>Delete?</h2><p>This will delete <strong data-delete-name>this item</strong>.</p>` +
		`<p class="page-copy" data-delete-description hidden></p><p class="error" data-delete-error></p>` +
		`<div class="delete-dialog-actions"><button class="or-button secondary" type="button" data-delete-cancel>Cancel</button>` +
		`<button class="or-button delete-confirm" type="button" data-delete-confirm>Delete</button></div></div></dialog>`
}
