pub(super) fn authorize_page(
    request: &str,
    client: &str,
    user: &str,
    callback: &str,
    scopes: &[serde_json::Value],
) -> String {
    let scopes = scopes
        .iter()
        .filter_map(|value| value.as_str())
        .map(|scope| {
            format!(
                "<label class=\"mcp-choice\"><input type=\"checkbox\" name=\"scope\" value=\"{}\" {}><span class=\"mcp-choice-copy\"><strong>{}</strong><small>{}</small></span></label>",
                esc(scope),
                if scope == "mcp:observe" { "checked" } else { "" },
                if scope == "mcp:terminal" { "Terminal" } else { "Observe" },
                if scope == "mcp:terminal" { "Run arbitrary commands. Command and output plaintext pass through RC." } else { "Machine status and metadata." }
            )
        })
        .collect::<String>();
    let body = format!(
        "<main class=\"auth-shell\" data-mcp-request=\"{}\"><div class=\"ohrats-grid auth-grid\"></div><div class=\"auth-content mcp-consent\"><p class=\"eyebrow\">OHRATS RC / MCP</p><h1>Connect {}</h1><div class=\"mcp-identity\"><span>Signed in as <strong>{}</strong></span><button class=\"text-button\" type=\"button\" data-mcp-switch-account>NOT YOU?</button></div><p class=\"page-copy\">Choose what this AI agent may do across your account. Access includes current and future machines in workspaces you can access. Terminal requires Owner access.</p><p class=\"meta\">OAUTH CALLBACK · <code>{}</code></p><form class=\"auth-form\" data-mcp-form><label>Access duration<select name=\"lifetime\"><option value=\"never\">Until revoked</option><option value=\"30d\">30 days</option></select></label><fieldset class=\"scope-fields\"><legend>Permissions</legend>{}</fieldset><div class=\"mcp-consent-actions\"><button class=\"or-button\" type=\"submit\">AUTHORIZE WITH PASSKEY</button><button class=\"or-button secondary\" type=\"button\" data-mcp-cancel>CANCEL</button></div><p class=\"muted\">Offline machines become available after reconnecting and accepting their permission update. Access can be revoked from RC at any time. OAuth access tokens remain short-lived and rotate independently.</p><p class=\"error\" data-mcp-error></p></form></div></main>",
        esc(request),
        esc(client),
        esc(user),
        esc(callback),
        scopes,
    );
    crate::page_html::public_document(
        &format!("Connect {client}"),
        body,
        &["mcp-authorize"],
        &[],
        "",
    )
}

fn esc(value: &str) -> String {
    value
        .replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
        .replace('"', "&quot;")
}
