use super::{PageContext, authenticated_document, esc, format, integer, string};

pub fn account(context: &PageContext, passkeys: &[serde_json::Value]) -> String {
    let passkey_rows: String = if passkeys.is_empty() {
        "<p class=\"empty-state\">No passkeys. This browser session is your remaining access.</p>"
            .into()
    } else {
        passkeys.iter().enumerate().map(|(index,passkey)|{
        let id=string(passkey,"id");let last=passkey.get("last_used").and_then(serde_json::Value::as_i64);format!("<div class=\"setting-row\"><div><strong>Passkey {}</strong><div class=\"meta\">{}</div></div>{}</div>",index+1,if last.is_some(){format!("USED {}",format::relative(last))}else{"NOT USED YET".into()},if passkeys.len()>1{format!("<form method=\"post\" action=\"/account/passkeys/{}/delete\"><button class=\"text-button\" type=\"submit\">REMOVE</button></form>",esc(&id))}else{"<span class=\"meta\" title=\"Add another passkey before removing this one.\">LAST PASSKEY</span>".into()})
    }).collect()
    };
    let body = format!(
        "<div class=\"page\"><header class=\"page-header account-header\"><div><div class=\"page-title-row\"><h1 data-account-name-view>{}</h1><form method=\"post\" action=\"/account/name\" hidden data-account-name-form><input class=\"account-title-input\" name=\"name\" value=\"{}\" aria-label=\"Account name\" required maxlength=\"120\"></form><button class=\"header-icon-button\" type=\"button\" data-account-rename aria-label=\"Rename account\" title=\"Rename account\"><span class=\"ui-icon icon-pencil\"></span></button></div><p class=\"error account-title-error\"></p></div><button class=\"header-icon-button danger-icon-button\" type=\"button\" aria-label=\"Delete account\" title=\"Delete account\" data-delete-kind=\"account\" data-delete-name=\"{}\" data-delete-description=\"Sole-owned workspaces are deleted. Shared workspaces are transferred to another Owner.\" data-delete-endpoint=\"/api/v1/account\" data-delete-method=\"DELETE\" data-delete-redirect=\"/\"><span class=\"ui-icon icon-trash\"></span></button></header><section class=\"content-section\"><div class=\"section-heading\"><div><div class=\"badge-container\"><span class=\"badge-text\">01 Passkeys</span><div class=\"badge-line\"></div></div><h2>Sign-in credentials</h2></div><button id=\"add-passkey\" class=\"text-button\" type=\"button\">ADD PASSKEY</button></div><div class=\"settings-list\">{}</div><p id=\"passkey-error\" class=\"error\"></p></section></div>",
        esc(&context.user.name),
        esc(&context.user.name),
        esc(&context.user.name),
        passkey_rows
    );
    authenticated_document(context, "Account", body, &["account"], &[])
}

pub fn api_keys(context: &PageContext, keys: &[serde_json::Value]) -> String {
    let rows: String = if keys.is_empty() {
        "<p class=\"empty-state\">No API keys yet.</p>".into()
    } else {
        keys.iter().map(key_row).collect()
    };
    let body = format!(
        "<div class=\"page\"><header class=\"page-header\"><div><h1>API</h1></div><div class=\"page-header-actions\"><a class=\"or-button\" href=\"/docs/api\">API DOCS <span aria-hidden=\"true\">→</span></a><button class=\"header-icon-button\" type=\"button\" aria-label=\"New API key\" title=\"New key\" data-api-key-new><span class=\"ui-icon icon-plus\"></span></button></div></header><div class=\"settings-list\" id=\"token-list\">{}</div>{}</div>",
        rows,
        key_dialog()
    );
    authenticated_document(context, "API access", body, &["api-page"], &[])
}

fn key_row(key: &serde_json::Value) -> String {
    let id = string(key, "id");
    let scopes = key
        .get("scopes")
        .and_then(serde_json::Value::as_array)
        .map(|values| {
            values
                .iter()
                .filter_map(serde_json::Value::as_str)
                .collect::<Vec<_>>()
                .join(" · ")
                .to_ascii_uppercase()
        })
        .unwrap_or_default();
    let last = key.get("last_used").and_then(serde_json::Value::as_i64);
    let expires = integer(key, "expires_at");
    format!(
        "<div class=\"setting-row token-row\"><div class=\"token-row-main\"><span class=\"ui-icon icon-key\"></span><div><strong>{}</strong><div class=\"meta\">{} · {} · {}</div></div></div><form method=\"post\" action=\"/api/tokens/{}/delete\"><button class=\"text-button\" type=\"submit\">REVOKE</button></form></div>",
        esc(&string(key, "name")),
        esc(&scopes),
        if last.is_some() {
            format!("USED {}", format::relative(last))
        } else {
            "NEVER USED".into()
        },
        if expires == 0 {
            "UNTIL REVOKED".into()
        } else {
            format!("EXPIRES IN {}", format::until(expires))
        },
        esc(&id)
    )
}

fn key_dialog() -> String {
    "<dialog class=\"form-dialog api-key-dialog\" data-api-key-dialog aria-labelledby=\"api-key-dialog-title\"><div class=\"dialog-content\" data-api-key-create><h2 id=\"api-key-dialog-title\">New API key</h2><form class=\"dialog-form\" data-api-key-form><label>Name<input name=\"name\" data-api-key-name placeholder=\"Automation\" required maxlength=\"80\"></label><label>Access duration<select name=\"lifetime\"><option value=\"never\">Until revoked</option><option value=\"30d\">30 days</option><option value=\"7d\">7 days</option></select></label><fieldset class=\"scope-fields\"><legend>Permissions</legend><label><input type=\"checkbox\" name=\"scope\" value=\"read\" checked> Read</label><label><input type=\"checkbox\" name=\"scope\" value=\"execute\" checked> Execute</label><label><input type=\"checkbox\" name=\"scope\" value=\"manage-devices\"> Manage devices</label><label><input type=\"checkbox\" name=\"scope\" value=\"manage-workspaces\"> Manage workspaces</label></fieldset><p class=\"error\" data-api-key-error></p><div class=\"dialog-actions\"><button class=\"or-button secondary\" type=\"button\" data-api-key-cancel>Cancel</button><button class=\"or-button\" type=\"submit\">Create</button></div></form></div><div class=\"dialog-content\" data-api-key-result hidden><div class=\"dialog-title-row\"><span class=\"ui-icon icon-key\"></span><h2>API signing key created</h2></div><p class=\"page-copy\">Copy this private signing key now. RC stores only its public key and cannot recover it.</p><div class=\"or-copy-field api-key-secret\"><code data-api-key-secret></code><button class=\"or-copy-button\" type=\"button\" aria-label=\"Copy API signing key\" data-api-key-copy><span class=\"or-copy-icon\"></span></button></div><div class=\"dialog-actions\"><button class=\"or-button\" type=\"button\" data-api-key-done>Done</button></div></div></dialog>".into()
}
