use super::{PageContext, authenticated_document, esc, format, public_document, string};

pub fn enroll(context: &PageContext, selected: &str) -> String {
    let owners = context
        .workspaces
        .iter()
        .filter(|workspace| string(workspace, "role") == "owner")
        .collect::<Vec<_>>();
    let options = owners
        .iter()
        .map(|workspace| {
            let id = string(workspace, "id");
            format!(
                "<option value=\"{}\" {}>{}</option>",
                esc(&id),
                if id == selected { "selected" } else { "" },
                esc(&string(workspace, "name"))
            )
        })
        .collect::<String>();
    let known = context
        .devices
        .iter()
        .filter(|device| selected.is_empty() || string(device, "workspace_id") == selected)
        .map(|device| string(device, "id"))
        .collect::<Vec<_>>()
        .join(",");
    let content = if owners.is_empty() {
        "<p class=\"empty-state\">You need to own a workspace before enrolling a device.</p>".into()
    } else {
        format!(
            "<form data-enrollment-form class=\"simple-form\"><label>Workspace<select name=\"workspaceId\">{}</select></label><button class=\"or-button\" type=\"submit\">CREATE ENROLLMENT COMMANDS</button></form><div class=\"enrollment-command\"><span class=\"meta\">NEW INSTALL · SHOWN ONCE</span><div class=\"or-copy-field enrollment-install\" data-enrollment-copy-field hidden><code data-enrollment-result=\"install\"></code><button class=\"or-copy-button\" type=\"button\" aria-label=\"Copy install command\" data-enrollment-copy><span class=\"or-copy-icon\" aria-hidden=\"true\"></span></button></div><span class=\"meta\">ALREADY INSTALLED · SHOWN ONCE</span><div class=\"or-copy-field enrollment-install\" data-enrollment-copy-field hidden><code data-enrollment-result=\"enroll\"></code><button class=\"or-copy-button\" type=\"button\" aria-label=\"Copy enroll command\" data-enrollment-copy><span class=\"or-copy-icon\" aria-hidden=\"true\"></span></button></div><p class=\"meta\">One default background enrollment is supported per OS user. RC refuses to overwrite an existing device identity.</p><p class=\"error\" role=\"alert\" data-enrollment-error></p><p id=\"enrollment-state\" class=\"meta\"></p></div>",
            options
        )
    };
    let body = format!(
        "<div class=\"page narrow-form-page\" data-enroll-page=\"{}\" data-known-devices=\"{}\"><header class=\"page-header\"><div><p class=\"eyebrow\">DEVICES</p><h1>Enroll device</h1></div></header>{}</div>",
        esc(selected),
        esc(&known),
        content
    );
    authenticated_document(context, "Enroll device", body, &["pages", "live"], &[])
}

pub fn cli_login(
    context: &PageContext,
    code: &str,
    client: &str,
    key: &str,
    lifetime: &str,
) -> String {
    let body = format!(
        "<main class=\"auth-shell\" data-cli-client=\"{}\" data-cli-public-key=\"{}\" data-cli-lifetime=\"{}\"><div class=\"ohrats-grid auth-grid\"></div><div class=\"auth-content\"><p class=\"eyebrow\">RC / COMMAND LINE</p><h1>Authorize CLI</h1><p class=\"page-copy\">Allow the RC command line to act as <strong>{}</strong> on your workspaces.</p><form method=\"post\" action=\"/cli/login\" class=\"auth-form\"><input type=\"hidden\" name=\"code\" value=\"{}\"><button class=\"or-button\" type=\"submit\">AUTHORIZE CLI</button></form><p class=\"error\"></p></div></main>",
        esc(client),
        esc(key),
        esc(lifetime),
        esc(&context.user.name),
        esc(code)
    );
    public_document("Authorize CLI", body, &["cli-authorize"], &[], "")
}

pub fn mcp_page(context: &PageContext, resource: &str, grants: &[super::McpPageGrant]) -> String {
    let rows: String = if grants.is_empty() {
        "<p class=\"empty-state\">No AI agents are connected yet. Add RC to your MCP client to start.</p>".into()
    } else {
        grants.iter().map(|grant|format!("<div class=\"setting-row\"><div><strong>{}</strong><div class=\"meta\">{} · {} MACHINE{} · {}</div></div><button class=\"text-button\" type=\"button\" data-mcp-revoke=\"{}\">REVOKE</button></div>",esc(&grant.name),esc(&grant.scopes.to_ascii_uppercase()),grant.device_count,if grant.device_count==1{""}else{"S"},if let Some(last)=grant.last_used{format!("USED {}",format::relative(Some(last)))}else if grant.expires_at==0{"UNTIL REVOKED".into()}else{format!("EXPIRES IN {}",format::until(grant.expires_at))},esc(&grant.id))).collect()
    };
    let body = format!(
        "<div class=\"page\"><header class=\"page-header\"><div><p class=\"eyebrow\">RC / MODEL CONTEXT PROTOCOL</p><h1>MCP</h1><p class=\"page-copy\">Connect an AI agent to <strong>RC</strong> with this URL: <code>{}</code></p><p class=\"meta\">CONFIG IDENTIFIER · <code>rc</code></p></div></header><div class=\"settings-list\">{}</div><p class=\"error\" data-mcp-page-error></p></div>",
        esc(resource),
        rows
    );
    authenticated_document(context, "MCP", body, &["mcp-page"], &[])
}

pub fn workspace_access(
    context: &PageContext,
    workspace: &serde_json::Value,
    members: &[serde_json::Value],
    invites: &[serde_json::Value],
) -> String {
    let id = string(workspace, "id");
    let name = string(workspace, "name");
    let member_rows=members.iter().map(|member|{let user_id=string(member,"user_id");let member_name=string(member,"name");let role=string(member,"role");format!("<div class=\"setting-row access-row\"><div><strong>{}{}</strong><div class=\"meta\">JOINED {}</div></div><div class=\"row-actions\"><form method=\"post\" action=\"/workspaces/{}/members/{}/role\" class=\"role-form\"><select name=\"role\" aria-label=\"Role for {}\"><option value=\"owner\" {}>Owner</option><option value=\"operator\" {}>Operator</option><option value=\"viewer\" {}>Viewer</option></select><button class=\"text-button\" type=\"submit\">SAVE</button></form>{}</div><p class=\"error row-error\"></p></div>",esc(&member_name),if user_id==context.user.id{" (you)"}else{""},esc(&format::relative(member.get("joined_at").and_then(serde_json::Value::as_i64))),esc(&id),esc(&user_id),esc(&member_name),selected(&role,"owner"),selected(&role,"operator"),selected(&role,"viewer"),if user_id!=context.user.id{format!("<form method=\"post\" action=\"/workspaces/{}/members/{}/remove\"><button class=\"text-button danger-text\" type=\"submit\">REMOVE</button></form>",esc(&id),esc(&user_id))}else{"".into()})}).collect::<String>();
    let invite_rows: String = if invites.is_empty() {
        "<p class=\"empty-state\">No pending invitations.</p>".into()
    } else {
        invites.iter().map(|invite|format!("<div class=\"setting-row\"><div><strong>{}</strong><div class=\"meta\">EXPIRES IN {}</div></div><form method=\"post\" action=\"/workspaces/{}/invites/{}/revoke\"><button class=\"text-button\" type=\"submit\">REVOKE</button></form></div>",esc(&string(invite,"role").to_ascii_uppercase()),esc(&format::until(invite.get("expires_at").and_then(serde_json::Value::as_i64).unwrap_or_default())),esc(&id),esc(&string(invite,"id")))).collect()
    };
    let member_noun = if members.len() == 1 {
        "person"
    } else {
        "people"
    };
    let body = format!(
        "<div class=\"page\" data-authority-workspace=\"{}\"><header class=\"page-header\"><div><p class=\"eyebrow\">{} / ACCESS</p><h1>Manage access</h1></div></header><section class=\"content-section\"><div class=\"section-heading\"><div><h2>Invite an operator or viewer</h2></div></div><form method=\"post\" action=\"/workspaces/{}/invites\" class=\"inline-form compact-inline-form\"><label>Role<select name=\"role\"><option value=\"operator\">Operator</option><option value=\"viewer\">Viewer</option></select></label><button class=\"or-button\" type=\"submit\">CREATE INVITE</button></form><div data-invite-result></div></section><section class=\"content-section\"><div class=\"section-heading\"><div><h2>{} {}</h2></div><button class=\"text-button\" type=\"button\" data-authority-sync=\"{}\">SYNC DEVICE ACCESS</button></div><p class=\"meta\" data-authority-status=\"{}\">RC Lock changes are accepted by Nodes only after Owner authorization.</p><div class=\"settings-list\">{}</div></section><section class=\"content-section\"><h2>Pending invitations</h2><div class=\"settings-list\">{}</div></section></div>",
        esc(&id),
        esc(&name.to_ascii_uppercase()),
        esc(&id),
        members.len(),
        member_noun,
        esc(&id),
        esc(&id),
        member_rows,
        invite_rows
    );
    authenticated_document(
        context,
        &format!("{name} access"),
        body,
        &["authority", "pages"],
        &[],
    )
}

pub fn activity(
    context: &PageContext,
    workspace: &serde_json::Value,
    events: &[serde_json::Value],
) -> String {
    let id = string(workspace, "id");
    let rows: String = if events.is_empty() {
        "<p class=\"empty-state\">No activity.</p>".into()
    } else {
        events.iter().map(|event|format!("<div class=\"activity-row\"><span>{}</span><span>{}</span><time>{}</time></div>",esc(&string(event,"kind").to_ascii_uppercase()),esc(&event_detail(event)),esc(&format::relative(event.get("created_at").and_then(serde_json::Value::as_i64))))).collect()
    };
    let body = format!(
        "<div class=\"page\" data-activity-page=\"{}\"><header class=\"page-header\"><div><p class=\"eyebrow\">{} / ACTIVITY</p><h1>Activity</h1></div></header><section class=\"content-section\"><div id=\"activity-list\" class=\"activity-list\">{}</div></section></div>",
        esc(&id),
        esc(&string(workspace, "name").to_ascii_uppercase()),
        rows
    );
    authenticated_document(context, "Activity", body, &["live"], &[])
}

pub fn error(status: u16, message: &str) -> String {
    public_document(
        &format!("Error {status}"),
        format!(
            "<main class=\"auth-shell\"><div class=\"auth-content\"><p class=\"eyebrow\">{status}</p><h1>{}</h1><a class=\"text-action\" href=\"/\">RETURN TO RC</a></div></main>",
            esc(message)
        ),
        &[],
        &[],
        "",
    )
}
fn selected(value: &str, want: &str) -> &'static str {
    if value == want { "selected" } else { "" }
}
fn event_detail(event: &serde_json::Value) -> String {
    let detail = event.get("detail").cloned().unwrap_or_default();
    ["name", "deviceId", "processId"]
        .iter()
        .find_map(|key| detail.get(*key).and_then(serde_json::Value::as_str))
        .or_else(|| event.get("device_id").and_then(serde_json::Value::as_str))
        .unwrap_or_default()
        .chars()
        .take(120)
        .collect()
}
