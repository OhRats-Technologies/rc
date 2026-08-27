use super::{PageContext, authenticated_document, bool_value, esc, format, integer, string};

pub fn devices(context: &PageContext) -> String {
    let rows = if context.devices.is_empty() {
        "<p class=\"empty-state\">No devices yet. Enroll one from a workspace.</p>".into()
    } else {
        context.devices.iter().map(device_row).collect::<String>()
    };
    let can_enroll = context
        .workspaces
        .iter()
        .any(|workspace| string(workspace, "role") == "owner");
    let body = format!(
        "<div class=\"page\" data-live-page=\"devices\"><header class=\"page-header\"><div><h1>Devices</h1></div>{}</header><div class=\"data-list\" id=\"device-list\">{}</div></div>",
        if can_enroll {
            "<a class=\"header-icon-button\" href=\"/devices/enroll\" aria-label=\"Enroll device\" title=\"Enroll device\"><span class=\"ui-icon icon-plus\"></span></a>"
        } else {
            ""
        },
        rows
    );
    authenticated_document(context, "Devices", body, &["live"], &[])
}

fn device_row(device: &serde_json::Value) -> String {
    let id = string(device, "id");
    let name = string(device, "name");
    let platform = string(device, "platform");
    let active = integer(device, "active_processes");
    let online = bool_value(device, "online");
    let status = if online {
        "ONLINE".into()
    } else {
        format!(
            "SEEN {}",
            format::relative(device.get("last_seen").and_then(serde_json::Value::as_i64))
        )
    };
    format!(
        "<a class=\"data-row\" href=\"/devices/{}\" data-device-row=\"{}\"><div class=\"device-row-main\"><span class=\"ui-icon device-platform-icon {}\"></span><div><strong>{}</strong><div class=\"meta\">{} · {}/{}{} </div></div></div><span class=\"status{}\" data-device-status=\"{}\">{}</span></a>",
        esc(&id),
        esc(&id),
        format::platform_icon(&platform),
        esc(&name),
        esc(&string(device, "workspace_name")),
        esc(&platform.to_ascii_uppercase()),
        esc(&string(device, "arch")),
        if active > 0 {
            format!(" · {active} ACTIVE")
        } else {
            String::new()
        },
        if online { " online" } else { "" },
        esc(&id),
        esc(&status)
    )
}

pub fn device(
    context: &PageContext,
    device: &serde_json::Value,
    processes: &[serde_json::Value],
) -> String {
    let id = string(device, "id");
    let name = string(device, "name");
    let role = string(device, "role");
    let platform = string(device, "platform");
    let online = bool_value(device, "online");
    let supports_process = device
        .get("capabilities")
        .and_then(serde_json::Value::as_array)
        .is_some_and(|values| values.iter().any(|value| value.as_str() == Some("process")));
    let can_operate = matches!(role.as_str(), "owner" | "operator");
    let can_manage = role == "owner";
    let process_rows: String = if role == "viewer" {
        "<p class=\"empty-state\">Process history is available to operators and owners.</p>".into()
    } else if processes.is_empty() {
        "<p class=\"empty-state\">No processes yet.</p>".into()
    } else {
        processes
            .iter()
            .map(|process| process_row(&id, process))
            .collect()
    };
    let rename = if can_manage {
        format!(
            "<form method=\"post\" action=\"/devices/{}/rename\" hidden data-device-title-form><input class=\"device-title-input\" name=\"name\" value=\"{}\" aria-label=\"Device name\" required maxlength=\"120\"><input type=\"hidden\" name=\"next\" value=\"/devices/{}\"></form><button class=\"header-icon-button\" type=\"button\" data-device-title-rename aria-label=\"Rename device\" title=\"Rename device\"><span class=\"ui-icon icon-pencil\"></span></button>",
            esc(&id),
            esc(&name),
            esc(&id)
        )
    } else {
        String::new()
    };
    let delete = if can_manage {
        format!(
            "<button class=\"header-icon-button danger-icon-button\" type=\"button\" aria-label=\"Delete {}\" title=\"Delete device\" data-delete-kind=\"device\" data-delete-name=\"{}\" data-delete-description=\"Deletes from RC immediately. If the Node is offline, it clears its old enrollment the next time it connects.\" data-delete-endpoint=\"/api/v1/devices/{}\" data-delete-redirect=\"/devices\"><span class=\"ui-icon icon-trash\"></span></button>",
            esc(&name),
            esc(&name),
            esc(&id)
        )
    } else {
        String::new()
    };
    let terminal = if can_operate {
        format!(
            "<button id=\"open-terminal\" class=\"device-terminal-button\" type=\"button\" aria-label=\"Open terminal\" title=\"Open terminal\" {}><span class=\"ui-icon icon-terminal\"></span></button>",
            if online && supports_process {
                ""
            } else {
                "disabled"
            }
        )
    } else {
        String::new()
    };
    let body = format!(
        "<div class=\"page\" data-device-page=\"{}\" data-supports-process=\"{}\"><section class=\"device-overview\"><header class=\"page-header device-header\"><div><p class=\"eyebrow\">DEVICE</p><div class=\"page-title-row device-title-row\"><span class=\"ui-icon device-platform-icon device-title-platform {}\"></span><h1 data-device-title-view>{}</h1>{}</div><p class=\"error device-title-error\" data-device-title-error></p><p class=\"meta\">{} · {}/{}</p></div><div class=\"device-header-actions\"><span id=\"device-status\" class=\"status{}\">{}</span>{}{}</div></header><dl class=\"facts\"><div><dt>HOST</dt><dd>{}</dd></div><div><dt>NODE VERSION</dt><dd id=\"node-version\">{}</dd></div></dl><p id=\"process-error\" class=\"error\">{}</p></section><section class=\"content-section\"><div class=\"section-heading\"><div><div class=\"badge-container\"><span class=\"badge-text\">01 Processes</span><div class=\"badge-line\"></div></div><h2>History</h2></div></div><div class=\"data-list\" id=\"process-list\">{}</div></section></div>",
        esc(&id),
        supports_process,
        format::platform_icon(&platform),
        esc(&name),
        rename,
        esc(&string(device, "workspace_name")),
        esc(&platform.to_ascii_uppercase()),
        esc(&string(device, "arch")),
        if online { " online" } else { "" },
        if online { "ONLINE" } else { "OFFLINE" },
        terminal,
        delete,
        esc(&string(device, "hostname")),
        esc(&string(device, "version")),
        if supports_process {
            ""
        } else {
            "This RC Node is too old for terminals. Run rc update on the device."
        },
        process_rows
    );
    authenticated_document(context, &name, body, &["live", "device"], &[])
}

fn process_row(device_id: &str, process: &serde_json::Value) -> String {
    let id = string(process, "id");
    let status = string(process, "status");
    format!(
        "<a class=\"data-row process-row\" href=\"/devices/{}/processes/{}\" data-process-row=\"{}\"><div><strong class=\"mono\">{}</strong><div class=\"meta\">{} · {} · {}</div></div><span class=\"status{}\" data-process-status=\"{}\">{}</span></a>",
        esc(device_id),
        esc(&id),
        esc(&id),
        esc(&format::process_label(process)),
        esc(&string(process, "origin").to_ascii_uppercase()),
        esc(&string(process, "created_by_name")),
        esc(&format::relative(
            process
                .get("created_at")
                .and_then(serde_json::Value::as_i64)
        )),
        if status == "running" { " online" } else { "" },
        esc(&id),
        esc(&format::process_state(process))
    )
}

pub fn process(
    context: &PageContext,
    device: &serde_json::Value,
    process: &serde_json::Value,
) -> String {
    let id = string(process, "id");
    let device_id = string(device, "id");
    let status = string(process, "status");
    let running = matches!(status.as_str(), "starting" | "running");
    let role = string(device, "role");
    let owner = string(process, "created_by");
    let controllable = role == "owner" || (role == "operator" && owner == context.user.id);
    let direct = matches!(
        string(process, "origin").as_str(),
        "browser" | "cli" | "api"
    );
    let terminal = bool_value(process, "terminal");
    let interactive = direct && terminal && running && controllable;
    let label = format::process_label(process);
    let terminal_body = if interactive {
        format!(
            "<pre id=\"process-transcript\" class=\"terminal-transcript\">Terminal scrollback is retained in RC Node memory while this process is live.</pre><div id=\"terminal-host\" class=\"terminal-host\" hidden></div><div class=\"mobile-terminal-keys\" aria-label=\"Terminal keys\">{}</div>",
            mobile_keys()
        )
    } else {
        format!(
            "<pre class=\"terminal-transcript\">{}</pre>",
            if terminal {
                "Terminal content is retained only in RC Node memory while the process is live."
            } else {
                "Process content is not retained by RC."
            }
        )
    };
    let actions = if interactive {
        "<div id=\"terminal-actions\" class=\"terminal-actions\"><button class=\"text-button\" data-signal=\"INT\" type=\"button\">CTRL-C</button><button class=\"text-button\" data-signal=\"TERM\" type=\"button\">TERM</button><button class=\"text-button\" data-signal=\"KILL\" type=\"button\">KILL</button></div>"
    } else {
        ""
    };
    let message = string(process, "error");
    let message = if message.is_empty() && running && !controllable {
        format!(
            "Live control belongs to {}.",
            string(process, "created_by_name")
        )
    } else {
        message
    };
    let body = format!(
        "<div class=\"page process-page\" data-process-page=\"{}\" data-device-id=\"{}\" data-process-status=\"{}\" data-process-live=\"{}\" data-process-interactive=\"{}\"><header class=\"page-header process-header\"><div><p class=\"eyebrow\"><a href=\"/devices/{}\">{}</a> / PROCESS</p><h1 class=\"mono process-title\">{}</h1><p class=\"meta\">{} · STARTED BY {} · {}</p></div><span id=\"process-state\" class=\"status{}\">{}</span></header><div id=\"terminal-toolbar\" class=\"terminal-toolbar\"><span class=\"terminal-label\">{}/{}{} </span>{}</div>{}<p id=\"process-message\" class=\"meta process-message\">{}</p></div>",
        esc(&id),
        esc(&device_id),
        esc(&status),
        running,
        interactive,
        esc(&device_id),
        esc(&string(device, "name").to_ascii_uppercase()),
        esc(&label),
        esc(&string(process, "origin").to_ascii_uppercase()),
        esc(&string(process, "created_by_name")),
        esc(&format::relative(
            process
                .get("created_at")
                .and_then(serde_json::Value::as_i64)
        )),
        if status == "running" { " online" } else { "" },
        esc(&format::process_state(process)),
        if terminal { "PTY" } else { "PROCESS" },
        esc(&id.chars().take(8).collect::<String>()),
        if interactive {
            " · <span id=\"control-transport\">CONNECTING</span>"
        } else {
            ""
        },
        actions,
        terminal_body,
        esc(&message)
    );
    authenticated_document(
        context,
        &label,
        body,
        if interactive {
            &["process-terminal"]
        } else {
            &[]
        },
        if interactive {
            &["process-terminal"]
        } else {
            &[]
        },
    )
}

fn mobile_keys() -> String {
    ["ESC", "CTRL", "ALT", "TAB", "LEFT", "UP", "DOWN", "RIGHT"]
        .iter()
        .map(|key| {
            format!(
                "<button type=\"button\" data-terminal-key=\"{}\">{}</button>",
                key,
                match *key {
                    "LEFT" => "←",
                    "UP" => "↑",
                    "DOWN" => "↓",
                    "RIGHT" => "→",
                    value => value,
                }
            )
        })
        .collect()
}
