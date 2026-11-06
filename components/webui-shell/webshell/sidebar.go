package webshell

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"wit_component/ohrats_rc_webui_types"
)

// renderSidebar builds the persistent authenticated navigation sidebar.
func renderSidebar(value ohrats_rc_webui_types.AuthenticatedDocument, additions string) string {
	var navigation strings.Builder
	for _, entry := range value.Navigation {
		navigation.WriteString(navLink(value.Path, entry))
	}
	var workspaces strings.Builder
	if len(value.Workspaces) == 0 {
		workspaces.WriteString(`<span class="sidebar-empty" data-workspace-empty>NO WORKSPACES</span>`)
	} else {
		for _, workspace := range value.Workspaces {
			workspaces.WriteString(workspaceFolder(value.Path, workspace))
		}
	}
	var builder strings.Builder
	builder.WriteString(`<aside id="site-sidebar" class="site-sidebar"><div class="sidebar-scroll"><a class="site-brand" href="/devices"><img src="` +
		sharedVisualAssets + `/logo.092a1cece4d0.svg" alt=""><strong>RC</strong></a><nav aria-label="RC navigation"><section class="sidebar-section"><h2>Navigation</h2>`)
	builder.WriteString(navigation.String())
	builder.WriteString(additions)
	builder.WriteString(`</section><section class="sidebar-section workspace-section"><div class="sidebar-section-title"><h2>Workspaces</h2><button class="workspace-add" type="button" aria-label="New workspace" title="New workspace" data-workspace-create-trigger><span class="ui-icon icon-plus" aria-hidden="true"></span></button></div><form class="workspace-create-form workspace-folder-head" method="post" action="/workspaces" hidden data-workspace-create-form><span class="ui-icon icon-folder" aria-hidden="true"></span><input class="workspace-create-input" name="name" aria-label="Workspace name" required maxlength="120"><input type="hidden" name="next" value="`)
	builder.WriteString(escape(value.Path))
	builder.WriteString(`"></form>`)
	builder.WriteString(workspaces.String())
	builder.WriteString(`</section></nav></div><div class="sidebar-footer"><div class="profile-row"><a class="profile-link" href="/account"><span class="profile-initial">`)
	builder.WriteString(escape(profileInitial(value.Principal.DisplayName)))
	builder.WriteString(`</span><span class="profile-name">`)
	builder.WriteString(escape(value.Principal.DisplayName))
	builder.WriteString(`</span></a><button class="theme-toggle" type="button" data-theme-toggle aria-label="Toggle theme"></button><form method="post" action="/account/logout"><button class="icon-button" type="submit" aria-label="Sign out" title="Sign out"><span class="ui-icon icon-sign-out"></span></button></form></div></div></aside><button id="sidebar-toggle" class="sidebar-toggle" type="button" aria-label="Toggle sidebar" data-sidebar-toggle><span class="ui-icon icon-sidebar"></span></button>`)
	return builder.String()
}

func navLink(current string, entry ohrats_rc_webui_types.NavigationEntry) string {
	active := current == entry.Path ||
		(entry.Path != "/api" && strings.HasPrefix(current, strings.TrimRight(entry.Path, "/")+"/"))
	class := ""
	if active {
		class = " active"
	}
	return fmt.Sprintf(`<a class="nav-link%s" href="%s" data-navigation-id="%s"><span class="ui-icon %s"></span><span>%s</span></a>`,
		class,
		escape(entry.Path),
		escape(entry.Id),
		escape(entry.Icon),
		escape(entry.Label),
	)
}

func workspaceFolder(path string, workspace ohrats_rc_webui_types.WorkspaceSummary) string {
	open := strings.HasPrefix(path, fmt.Sprintf("/workspaces/%s/", workspace.Id))
	children := `<span class="workspace-empty">No devices</span>`
	if workspace.DeviceCount != 0 {
		plural := "s"
		if workspace.DeviceCount == 1 {
			plural = ""
		}
		children = fmt.Sprintf(`<span class="workspace-empty">%d device%s</span>`, workspace.DeviceCount, plural)
	}
	active := ""
	if open {
		active = " active"
	}
	hidden := ""
	if !open {
		hidden = "hidden"
	}
	return fmt.Sprintf(`<div class="workspace-folder%s" data-workspace-folder="%s" data-default-open="%t"><div class="workspace-folder-head"><button class="workspace-toggle" type="button" aria-expanded="%t" data-workspace-toggle="%s"><span class="ui-icon icon-folder"></span><span class="workspace-name">%s</span></button></div><div class="workspace-children" data-workspace-children="%s" data-open="%t" %s>%s</div></div>`,
		active,
		escape(workspace.Id),
		open,
		open,
		escape(workspace.Id),
		escape(workspace.Name),
		escape(workspace.Id),
		open,
		hidden,
		children,
	)
}

func profileInitial(displayName string) string {
	trimmed := strings.TrimSpace(displayName)
	if trimmed == "" {
		return "?"
	}
	first, _ := utf8.DecodeRuneInString(trimmed)
	return strings.ToUpper(string(first))
}
