use super::{PageContext, bool_value, esc, string};

pub(super) fn render(context: &PageContext) -> String {
    let current_device = context
        .path
        .strip_prefix("/devices/")
        .and_then(|rest| rest.split('/').next())
        .unwrap_or_default();
    let current_workspace = context
        .devices
        .iter()
        .find(|device| string(device, "id") == current_device)
        .map(|device| string(device, "workspace_id"))
        .or_else(|| {
            context
                .path
                .strip_prefix("/workspaces/")
                .map(|rest| rest.split('/').next().unwrap_or_default().to_owned())
        })
        .unwrap_or_default();
    let folders = context
        .workspaces
        .iter()
        .map(|workspace| {
            let workspace_id = string(workspace, "id");
            let devices = context
                .devices
                .iter()
                .filter(|device| string(device, "workspace_id") == workspace_id)
                .collect::<Vec<_>>();
            workspace_folder(
                context,
                workspace,
                &devices,
                current_device,
                current_workspace == workspace_id,
            )
        })
        .collect::<String>();
    let initial = context
        .user
        .name
        .trim()
        .chars()
        .next()
        .unwrap_or('?')
        .to_uppercase()
        .collect::<String>();
    format!(
        "<aside id=\"site-sidebar\" class=\"site-sidebar\"><div class=\"sidebar-scroll\"><a class=\"site-brand\" href=\"/devices\"><img src=\"https://assets.ohrats.party/assets/logo.092a1cece4d0.svg\" alt=\"\"><strong>RC</strong></a><nav aria-label=\"RC navigation\"><section class=\"sidebar-section\"><h2>Navigation</h2>{}{}{} </section><section class=\"sidebar-section workspace-section\"><div class=\"sidebar-section-title\"><h2>Workspaces</h2><button class=\"workspace-add\" type=\"button\" aria-label=\"New workspace\" title=\"New workspace\" data-workspace-create-trigger><span class=\"ui-icon icon-plus\" aria-hidden=\"true\"></span></button></div><form class=\"workspace-create-form workspace-folder-head\" method=\"post\" action=\"/workspaces\" hidden data-workspace-create-form><span class=\"ui-icon icon-folder\" aria-hidden=\"true\"></span><input class=\"workspace-create-input\" name=\"name\" aria-label=\"Workspace name\" required maxlength=\"120\"><input type=\"hidden\" name=\"next\" value=\"{}\"></form>{}</section></nav></div><div class=\"sidebar-footer\"><div class=\"profile-row\"><a class=\"profile-link\" href=\"/account\"><span class=\"profile-initial\">{}</span><span class=\"profile-name\">{}</span></a><button class=\"theme-toggle\" type=\"button\" data-theme-toggle aria-label=\"Toggle theme\"></button><form method=\"post\" action=\"/account/logout\"><button class=\"icon-button\" type=\"submit\" aria-label=\"Sign out\" title=\"Sign out\"><span class=\"ui-icon icon-sign-out\"></span></button></form></div></div></aside>{}<button id=\"sidebar-toggle\" class=\"sidebar-toggle\" type=\"button\" aria-label=\"Toggle sidebar\"><span class=\"ui-icon icon-sidebar\"></span></button>",
        nav_link(&context.path, "/devices", "icon-devices", "Devices"),
        nav_link(&context.path, "/api", "icon-api", "API"),
        nav_link(&context.path, "/integrations/mcp", "icon-api", "MCP"),
        esc(&context.path),
        if folders.is_empty() {
            "<span class=\"sidebar-empty\" data-workspace-empty>NO WORKSPACES</span>".to_owned()
        } else {
            folders
        },
        esc(&initial),
        esc(&context.user.name),
        delete_dialog()
    )
}

fn nav_link(path: &str, target: &str, icon: &str, label: &str) -> String {
    let active = if path == target || (target != "/api" && path.starts_with(&format!("{target}/")))
    {
        " active"
    } else {
        ""
    };
    format!(
        "<a class=\"nav-link{}\" href=\"{}\"><span class=\"ui-icon {}\"></span><span>{}</span></a>",
        active,
        esc(target),
        esc(icon),
        esc(label)
    )
}

fn workspace_folder(
    context: &PageContext,
    workspace: &serde_json::Value,
    devices: &[&serde_json::Value],
    current_device: &str,
    open: bool,
) -> String {
    let id = string(workspace, "id");
    let name = string(workspace, "name");
    let role = string(workspace, "role");
    let owner = role == "owner";
    let device_rows = devices
        .iter()
        .enumerate()
        .map(|(index, device)| device_row(context, device, current_device, owner, index >= 5))
        .collect::<String>();
    let children = if device_rows.is_empty() {
        "<span class=\"workspace-empty\">No devices</span>".to_owned()
    } else {
        format!(
            "{}{}",
            device_rows,
            if devices.len() > 5 {
                format!(
                    "<button class=\"workspace-show-more\" type=\"button\" data-workspace-show-more=\"{}\">Show more</button>",
                    esc(&id)
                )
            } else {
                String::new()
            }
        )
    };
    format!(
        "<div class=\"workspace-folder{}\" data-workspace-folder=\"{}\" data-default-open=\"{}\"><div class=\"workspace-folder-head has-menu\"><button class=\"workspace-toggle\" type=\"button\" aria-expanded=\"{}\" data-workspace-toggle=\"{}\" data-workspace-name-view><span class=\"ui-icon icon-folder\"></span><span class=\"workspace-name\">{}</span></button>{}<details class=\"workspace-menu\"><summary class=\"workspace-menu-trigger\" aria-label=\"Actions for {}\" title=\"Workspace actions\"><span class=\"ui-icon icon-ellipsis\"></span></summary><div class=\"workspace-menu-popover\"><div class=\"workspace-menu-actions\">{}{}<a href=\"/workspaces/{}/activity\"><span class=\"ui-icon icon-audit\"></span>Audit log</a>{}{}</div></div></details></div><div class=\"workspace-children\" data-workspace-children=\"{}\" data-open=\"{}\" {}>{}</div></div>",
        if open { " active" } else { "" },
        esc(&id),
        open,
        open,
        esc(&id),
        esc(&name),
        if owner {
            format!(
                "<form class=\"workspace-inline-rename\" method=\"post\" action=\"/workspaces/{}/rename\" hidden data-workspace-rename-form><span class=\"ui-icon icon-folder\"></span><input class=\"workspace-rename-input\" name=\"name\" value=\"{}\" aria-label=\"Rename {}\" required maxlength=\"120\"><input type=\"hidden\" name=\"next\" value=\"{}\"></form>",
                esc(&id),
                esc(&name),
                esc(&name),
                esc(&context.path)
            )
        } else {
            String::new()
        },
        esc(&name),
        if owner {
            format!(
                "<a href=\"/devices/enroll?workspace={}\"><span class=\"ui-icon icon-enroll\"></span>Enroll device</a><a href=\"/workspaces/{}/access\"><span class=\"ui-icon icon-access\"></span>Manage access</a>",
                esc(&id),
                esc(&id)
            )
        } else {
            String::new()
        },
        "",
        esc(&id),
        if owner {
            "<button type=\"button\" data-workspace-rename><span class=\"ui-icon icon-pencil\"></span>Rename workspace</button>".to_owned()
        } else {
            String::new()
        },
        if owner {
            format!(
                "<button class=\"danger-text\" type=\"button\" data-delete-kind=\"workspace\" data-delete-name=\"{}\" data-delete-endpoint=\"/api/v1/workspaces/{}\"><span class=\"ui-icon icon-trash\"></span>Delete workspace</button>",
                esc(&name),
                esc(&id)
            )
        } else {
            format!(
                "<form method=\"post\" action=\"/workspaces/{}/leave\"><button type=\"submit\">Leave workspace</button></form>",
                esc(&id)
            )
        },
        esc(&id),
        open,
        if open { "" } else { "hidden" },
        children
    )
}

fn device_row(
    context: &PageContext,
    device: &serde_json::Value,
    current: &str,
    owner: bool,
    overflow: bool,
) -> String {
    let id = string(device, "id");
    let name = string(device, "name");
    let online = bool_value(device, "online");
    let capabilities = device
        .get("capabilities")
        .and_then(serde_json::Value::as_array)
        .cloned()
        .unwrap_or_default();
    let update = owner
        && capabilities
            .iter()
            .any(|value| value.as_str() == Some("update"));
    format!(
        "<div class=\"workspace-device-row{}{}\" data-sidebar-device=\"{}\" {}><div class=\"workspace-device-head{}\"><a class=\"workspace-device-link\" href=\"/devices/{}\" data-device-name-view><span class=\"workspace-device-presence{}\" data-sidebar-device-status=\"{}\"></span><span class=\"workspace-device-name\"><span>{}</span></span></a>{}<details class=\"workspace-menu device-menu\"><summary class=\"workspace-menu-trigger\" aria-label=\"Actions for {}\" title=\"Device actions\"><span class=\"ui-icon icon-ellipsis\"></span></summary><div class=\"workspace-menu-popover\"><div class=\"workspace-menu-actions\">{}{}{} </div></div></details></div></div>",
        if id == current { " active" } else { "" },
        if overflow {
            " workspace-device-overflow"
        } else {
            ""
        },
        esc(&id),
        if overflow { "hidden" } else { "" },
        if owner { " has-menu" } else { "" },
        esc(&id),
        if online { " online" } else { "" },
        esc(&id),
        esc(&name),
        if owner {
            format!(
                "<form class=\"device-inline-rename\" method=\"post\" action=\"/devices/{}/rename\" hidden data-device-rename-form><span class=\"workspace-device-presence{}\" data-sidebar-device-status=\"{}\"></span><input class=\"device-rename-input\" name=\"name\" value=\"{}\" aria-label=\"Rename {}\" required maxlength=\"120\"><input type=\"hidden\" name=\"next\" value=\"{}\"></form>",
                esc(&id),
                if online { " online" } else { "" },
                esc(&id),
                esc(&name),
                esc(&name),
                esc(&context.path)
            )
        } else {
            String::new()
        },
        esc(&name),
        if owner {
            "<button type=\"button\" data-device-rename><span class=\"ui-icon icon-pencil\"></span>Rename device</button>".to_owned()
        } else {
            String::new()
        },
        if update {
            format!(
                "<button type=\"button\" data-sidebar-device-update=\"{}\" {}>Update node</button>",
                esc(&id),
                if online { "" } else { "disabled" }
            )
        } else {
            String::new()
        },
        if owner {
            format!(
                "<button class=\"danger-text\" type=\"button\" data-delete-kind=\"device\" data-delete-name=\"{}\" data-delete-description=\"Deletes from RC immediately. If the Node is offline, it clears its old enrollment the next time it connects.\" data-delete-endpoint=\"/api/v1/devices/{}\"><span class=\"ui-icon icon-trash\"></span>Delete device</button>",
                esc(&name),
                esc(&id)
            )
        } else {
            String::new()
        }
    )
}

fn delete_dialog() -> String {
    "<dialog class=\"delete-dialog\" data-delete-dialog aria-labelledby=\"delete-dialog-title\"><div class=\"delete-dialog-content\"><h2 id=\"delete-dialog-title\" data-delete-title>Delete?</h2><p>This will delete <strong data-delete-name>this item</strong>.</p><p class=\"page-copy\" data-delete-description hidden></p><p class=\"error\" data-delete-error></p><div class=\"delete-dialog-actions\"><button class=\"or-button secondary\" type=\"button\" data-delete-cancel>Cancel</button><button class=\"or-button delete-confirm\" type=\"button\" data-delete-confirm>Delete</button></div></div></dialog>".to_owned()
}
