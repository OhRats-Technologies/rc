package server

import "strings"

type sidebarWorkspace struct {
	ID      string
	Name    string
	Role    string
	Devices []*RegisteredDevice
}

func (s *Server) renderSidebar(userID, userName, path string) string {
	currentDevice := ""
	if rest, ok := strings.CutPrefix(path, "/devices/"); ok {
		currentDevice = strings.SplitN(rest, "/", 2)[0]
	}
	devices := s.visibleDevices(userID)
	workspaces := make([]sidebarWorkspace, 0)
	index := make(map[string]int)
	if s.auth != nil {
		if access, err := s.auth.WorkspacesForUser(userID); err == nil {
			for _, workspace := range access {
				index[workspace.ID] = len(workspaces)
				workspaces = append(workspaces, sidebarWorkspace{
					ID: workspace.ID, Name: workspace.Name, Role: workspace.Role,
				})
			}
		}
	}
	currentWorkspace := ""
	for _, device := range devices {
		position, ok := index[device.WorkspaceID]
		if !ok {
			name := "Personal"
			if device.WorkspaceID != defaultWorkspaceID {
				name = device.WorkspaceID
			}
			position = len(workspaces)
			index[device.WorkspaceID] = position
			workspaces = append(workspaces, sidebarWorkspace{ID: device.WorkspaceID, Name: name, Role: "owner"})
		}
		workspaces[position].Devices = append(workspaces[position].Devices, device)
		if device.ID == currentDevice {
			currentWorkspace = device.WorkspaceID
		}
	}
	if rest, ok := strings.CutPrefix(path, "/workspaces/"); ok {
		currentWorkspace = strings.SplitN(rest, "/", 2)[0]
	}
	var folders strings.Builder
	for _, workspace := range workspaces {
		open := workspace.ID == currentWorkspace || (currentWorkspace == "" && strings.HasPrefix(path, "/devices"))
		folders.WriteString(s.sidebarWorkspaceFolder(workspace, currentDevice, path, currentWorkspace, open))
	}
	empty := `<span class="sidebar-empty" data-workspace-empty>NO WORKSPACES</span>`
	if len(workspaces) > 0 {
		empty = `<span class="sidebar-empty" data-workspace-empty hidden>NO WORKSPACES</span>`
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
		`</section><section class="sidebar-section workspace-section"><div class="sidebar-section-title"><h2>Workspaces</h2>` +
		`<button class="workspace-add" type="button" aria-label="New workspace" title="New workspace" data-workspace-create-trigger><span class="ui-icon icon-plus"></span></button></div>` +
		`<form class="workspace-create-form workspace-folder-head" method="post" action="/workspaces" hidden data-workspace-create-form>` +
		`<span class="ui-icon icon-folder"></span><input class="workspace-create-input" name="name" aria-label="Workspace name" required maxlength="120">` +
		`<input type="hidden" name="next" value="` + escape(path) + `"></form>` +
		folders.String() + empty + `</section></nav></div><div class="sidebar-footer"><div class="profile-row">` +
		`<a class="profile-link" href="/account"><span class="profile-initial">` + escape(initial) + `</span><span class="profile-name">` + escape(userName) + `</span></a>` +
		`<button class="theme-toggle" type="button" data-theme-toggle aria-label="Toggle theme"></button>` +
		`<form method="post" action="/account/logout"><button class="icon-button" type="submit" aria-label="Sign out" title="Sign out"><span class="ui-icon icon-sign-out"></span></button></form>` +
		`</div></div></aside><button id="sidebar-toggle" class="sidebar-toggle" type="button" aria-label="Toggle sidebar"><span class="ui-icon icon-sidebar"></span></button>` +
		deleteDialog()
}

func (s *Server) sidebarWorkspaceFolder(workspace sidebarWorkspace, currentDevice, path, currentWorkspace string, open bool) string {
	owner := workspace.Role == "owner"
	renameForm := ""
	if owner {
		renameForm = `<form class="workspace-inline-rename" method="post" action="/workspaces/` + escape(workspace.ID) +
			`/rename" hidden data-workspace-rename-form><span class="ui-icon icon-folder"></span>` +
			`<input class="workspace-rename-input" name="name" value="` + escape(workspace.Name) + `" aria-label="Rename ` +
			escape(workspace.Name) + `" required maxlength="120"><input type="hidden" name="next" value="` + escape(path) + `"></form>`
	}
	actions := `<a href="/workspaces/` + escape(workspace.ID) + `/activity"><span class="ui-icon icon-audit"></span>Activity</a>`
	if owner {
		actions = `<a href="/devices/enroll?workspace=` + escape(workspace.ID) + `"><span class="ui-icon icon-enroll"></span>Enroll device</a>` +
			`<a href="/workspaces/` + escape(workspace.ID) + `/access"><span class="ui-icon icon-access"></span>Manage access</a>` + actions +
			`<button type="button" data-workspace-rename><span class="ui-icon icon-pencil"></span>Rename workspace</button>` +
			`<button class="danger-text" type="button" data-delete-kind="workspace" data-delete-name="` + escape(workspace.Name) +
			`" data-delete-endpoint="/api/v1/workspaces/` + escape(workspace.ID) + `" data-delete-redirect="/devices"><span class="ui-icon icon-trash"></span>Delete workspace</button>`
	} else {
		actions += `<form method="post" action="/workspaces/` + escape(workspace.ID) + `/leave"><button type="submit"><span class="ui-icon icon-sign-out"></span>Leave workspace</button></form>`
	}
	var children strings.Builder
	for position, device := range workspace.Devices {
		children.WriteString(s.sidebarDeviceRow(device, currentDevice, position >= 5))
	}
	if len(workspace.Devices) == 0 {
		children.WriteString(`<span class="workspace-empty">No devices</span>`)
	}
	if len(workspace.Devices) > 5 {
		children.WriteString(`<button class="workspace-show-more" type="button" data-workspace-show-more="` + escape(workspace.ID) + `">Show more</button>`)
	}
	hidden := ""
	if !open {
		hidden = "hidden"
	}
	active := ""
	if workspace.ID == currentWorkspace {
		active = " active"
	}
	return `<div class="workspace-folder` + active + `" data-workspace-folder="` + escape(workspace.ID) +
		`" data-default-open="` + boolString(open) + `"><div class="workspace-folder-head has-menu">` +
		`<button class="workspace-toggle" type="button" aria-expanded="` + boolString(open) + `" data-workspace-toggle="` +
		escape(workspace.ID) + `" data-workspace-name-view><span class="ui-icon icon-folder"></span><span class="workspace-name">` +
		escape(workspace.Name) + `</span></button>` + renameForm + `<details class="workspace-menu"><summary class="workspace-menu-trigger" aria-label="Actions for ` +
		escape(workspace.Name) + `"><span class="ui-icon icon-ellipsis"></span></summary><div class="workspace-menu-popover"><div class="workspace-menu-actions">` +
		actions + `</div></div></details></div><div class="workspace-children" data-workspace-children="` + escape(workspace.ID) +
		`" data-open="` + boolString(open) + `" ` + hidden + `>` + children.String() + `</div></div>`
}

func (s *Server) sidebarDeviceRow(device *RegisteredDevice, current string, overflow bool) string {
	active, overflowClass, hidden := "", "", ""
	if device.ID == current {
		active = " active"
	}
	if overflow {
		overflowClass, hidden = " workspace-device-overflow", " hidden"
	}
	online, status := "", "OFFLINE"
	if device.Online {
		online, status = " online", "ONLINE"
	}
	_ = status
	return `<div class="workspace-device-row` + active + overflowClass + `" data-sidebar-device="` + escape(device.ID) + `"` + hidden + `>` +
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

func activeClass(path, target string) string {
	if path == target || strings.HasPrefix(path, target+"/") {
		return " active"
	}
	return ""
}
