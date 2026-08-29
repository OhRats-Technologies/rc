use crate::document::escape;
use crate::ohrats::rc_webui::types::{AuthenticatedDocument, NavigationEntry, WorkspaceSummary};

pub fn render(value: &AuthenticatedDocument, additions: &str) -> String {
    let navigation = value
        .navigation
        .iter()
        .map(|entry| nav_link(&value.path, entry))
        .collect::<String>();
    let workspaces: String = if value.workspaces.is_empty() {
        "<span class=\"sidebar-empty\" data-workspace-empty>NO WORKSPACES</span>".into()
    } else {
        value
            .workspaces
            .iter()
            .map(|workspace| workspace_folder(&value.path, workspace))
            .collect()
    };
    let initial = value
        .principal
        .display_name
        .trim()
        .chars()
        .next()
        .unwrap_or('?')
        .to_uppercase()
        .collect::<String>();
    format!(
        "<aside id=\"site-sidebar\" class=\"site-sidebar\"><div class=\"sidebar-scroll\"><a class=\"site-brand\" href=\"/devices\"><img src=\"https://assets.ohrats.party/assets/logo.092a1cece4d0.svg\" alt=\"\"><strong>RC</strong></a><nav aria-label=\"RC navigation\"><section class=\"sidebar-section\"><h2>Navigation</h2>{navigation}{additions}</section><section class=\"sidebar-section workspace-section\"><div class=\"sidebar-section-title\"><h2>Workspaces</h2><button class=\"workspace-add\" type=\"button\" aria-label=\"New workspace\" title=\"New workspace\" data-workspace-create-trigger><span class=\"ui-icon icon-plus\" aria-hidden=\"true\"></span></button></div><form class=\"workspace-create-form workspace-folder-head\" method=\"post\" action=\"/workspaces\" hidden data-workspace-create-form><span class=\"ui-icon icon-folder\" aria-hidden=\"true\"></span><input class=\"workspace-create-input\" name=\"name\" aria-label=\"Workspace name\" required maxlength=\"120\"><input type=\"hidden\" name=\"next\" value=\"{}\"></form>{workspaces}</section></nav></div><div class=\"sidebar-footer\"><div class=\"profile-row\"><a class=\"profile-link\" href=\"/account\"><span class=\"profile-initial\">{}</span><span class=\"profile-name\">{}</span></a><button class=\"theme-toggle\" type=\"button\" data-theme-toggle aria-label=\"Toggle theme\"></button><form method=\"post\" action=\"/account/logout\"><button class=\"icon-button\" type=\"submit\" aria-label=\"Sign out\" title=\"Sign out\"><span class=\"ui-icon icon-sign-out\"></span></button></form></div></div></aside><button id=\"sidebar-toggle\" class=\"sidebar-toggle\" type=\"button\" aria-label=\"Toggle sidebar\" data-sidebar-toggle><span class=\"ui-icon icon-sidebar\"></span></button>",
        escape(&value.path),
        escape(&initial),
        escape(&value.principal.display_name),
    )
}

fn nav_link(current: &str, entry: &NavigationEntry) -> String {
    let active = current == entry.path
        || (entry.path != "/api"
            && current.starts_with(&format!("{}/", entry.path.trim_end_matches('/'))));
    format!(
        "<a class=\"nav-link{}\" href=\"{}\" data-navigation-id=\"{}\"><span class=\"ui-icon {}\"></span><span>{}</span></a>",
        if active { " active" } else { "" },
        escape(&entry.path),
        escape(&entry.id),
        escape(&entry.icon),
        escape(&entry.label),
    )
}

fn workspace_folder(path: &str, workspace: &WorkspaceSummary) -> String {
    let open = path.starts_with(&format!("/workspaces/{}/", workspace.id));
    let children = if workspace.device_count == 0 {
        "<span class=\"workspace-empty\">No devices</span>".into()
    } else {
        format!(
            "<span class=\"workspace-empty\">{} device{}</span>",
            workspace.device_count,
            if workspace.device_count == 1 { "" } else { "s" }
        )
    };
    format!(
        "<div class=\"workspace-folder{}\" data-workspace-folder=\"{}\" data-default-open=\"{open}\"><div class=\"workspace-folder-head\"><button class=\"workspace-toggle\" type=\"button\" aria-expanded=\"{open}\" data-workspace-toggle=\"{}\"><span class=\"ui-icon icon-folder\"></span><span class=\"workspace-name\">{}</span></button></div><div class=\"workspace-children\" data-workspace-children=\"{}\" data-open=\"{open}\" {}>{children}</div></div>",
        if open { " active" } else { "" },
        escape(&workspace.id),
        escape(&workspace.id),
        escape(&workspace.name),
        escape(&workspace.id),
        if open { "" } else { "hidden" },
    )
}

#[cfg(test)]
mod tests {
    use super::{nav_link, render};
    use crate::ohrats::rc_webui::types::{
        AuthenticatedDocument, NavigationEntry, Principal, SidebarState, WorkspaceSummary,
    };
    #[test]
    fn navigation_marks_exact_and_nested_paths_active() {
        let entry = NavigationEntry {
            id: "devices".into(),
            label: "Devices".into(),
            path: "/devices".into(),
            icon: "icon-devices".into(),
        };
        assert!(nav_link("/devices/node", &entry).contains("nav-link active"));
        assert!(!nav_link("/account", &entry).contains("nav-link active"));
    }
    #[test]
    fn composes_navigation_workspaces_and_sidebar_slot() {
        let value = AuthenticatedDocument {
            title: "Devices".into(),
            principal: Principal {
                user_id: "u".into(),
                display_name: "River".into(),
            },
            path: "/devices".into(),
            sidebar: SidebarState::Open,
            navigation: vec![NavigationEntry {
                id: "devices".into(),
                label: "Devices".into(),
                path: "/devices".into(),
                icon: "icon-devices".into(),
            }],
            workspaces: vec![WorkspaceSummary {
                id: "w".into(),
                name: "Lab".into(),
                role: "owner".into(),
                device_count: 0,
            }],
            trusted_body: String::new(),
            scripts: vec![],
            styles: vec![],
        };
        let html = render(&value, "<span data-test-slot>slot</span>");
        for marker in [
            "nav-link active",
            "data-workspace-folder=\"w\"",
            "No devices",
            "data-test-slot",
        ] {
            assert!(html.contains(marker), "missing {marker}");
        }
    }
}
