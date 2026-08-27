use super::{MCP_SCOPES, mcp_resource};
use crate::{AppState, UserIdentity, now_ms, opaque};
use rusqlite::OptionalExtension;
use uuid::Uuid;

pub fn register_mcp_client(
    state: &AppState,
    input: &serde_json::Value,
) -> anyhow::Result<serde_json::Value> {
    let redirects = input
        .get("redirect_uris")
        .and_then(|value| value.as_array())
        .ok_or_else(|| anyhow::anyhow!("invalid redirect_uris"))?;
    let mut redirect_uris = Vec::new();
    for value in redirects.iter().take(10) {
        let uri = value
            .as_str()
            .ok_or_else(|| anyhow::anyhow!("invalid redirect_uris"))?;
        if !safe_redirect(uri) {
            anyhow::bail!("invalid redirect_uris");
        }
        if !redirect_uris.iter().any(|existing| existing == uri) {
            redirect_uris.push(uri.to_owned());
        }
    }
    if redirect_uris.is_empty() {
        anyhow::bail!("invalid redirect_uris");
    }
    let application_type = input
        .get("application_type")
        .and_then(|value| value.as_str())
        .unwrap_or("native");
    if !matches!(application_type, "native" | "web") {
        anyhow::bail!("invalid application_type");
    }
    if application_type == "web"
        && redirect_uris.iter().any(|uri| {
            url::Url::parse(uri)
                .ok()
                .is_none_or(|url| url.scheme() != "https")
        })
    {
        anyhow::bail!("web MCP clients require HTTPS redirect URIs");
    }
    if input
        .get("token_endpoint_auth_method")
        .and_then(|value| value.as_str())
        .unwrap_or("none")
        != "none"
    {
        anyhow::bail!("only public MCP clients are supported");
    }
    let id = format!("mcp_client_{}", opaque(18));
    let name = input
        .get("client_name")
        .and_then(|value| value.as_str())
        .unwrap_or("MCP client")
        .trim()
        .chars()
        .take(120)
        .collect::<String>();
    let name = if name.is_empty() {
        "MCP client".to_owned()
    } else {
        name
    };
    let redirect_uris_json = serde_json::to_string(&redirect_uris)?;
    let created_at = now_ms();
    state.db.with_connection(|db| {
        db.execute(
            "INSERT INTO mcp_clients(id,name,redirect_uris,created_at) VALUES(?,?,?,?)",
            rusqlite::params![id, name, redirect_uris_json, created_at],
        )?;
        Ok(())
    })?;
    Ok(serde_json::json!({
        "client_id":id,"client_id_issued_at":created_at/1000,"client_name":name,
        "redirect_uris":redirect_uris,"application_type":application_type,
        "token_endpoint_auth_method":"none","grant_types":["authorization_code","refresh_token"],
        "response_types":["code"]
    }))
}

pub fn create_oauth_request(
    state: &AppState,
    user: &UserIdentity,
    query: &str,
) -> anyhow::Result<serde_json::Value> {
    let params = url::form_urlencoded::parse(query.as_bytes())
        .into_owned()
        .collect::<std::collections::HashMap<_, _>>();
    let client_id = params.get("client_id").cloned().unwrap_or_default();
    let client =
        client_row(state, &client_id)?.ok_or_else(|| anyhow::anyhow!("unknown MCP client"))?;
    let redirect = params.get("redirect_uri").cloned().unwrap_or_default();
    let registered: Vec<String> = serde_json::from_str(&client.1)?;
    if !registered.contains(&redirect) {
        anyhow::bail!("redirect_uri is not registered");
    }
    if params.get("response_type").map(String::as_str) != Some("code") {
        anyhow::bail!("response_type must be code");
    }
    if params.get("code_challenge_method").map(String::as_str) != Some("S256") {
        anyhow::bail!("PKCE S256 is required");
    }
    let challenge = params.get("code_challenge").cloned().unwrap_or_default();
    if !(43..=128).contains(&challenge.len())
        || !challenge
            .chars()
            .all(|c| c.is_ascii_alphanumeric() || matches!(c, '-' | '_'))
    {
        anyhow::bail!("invalid PKCE code challenge");
    }
    let resource = params.get("resource").cloned().unwrap_or_default();
    if resource != mcp_resource(state) {
        anyhow::bail!("resource must identify this MCP server");
    }
    let scopes = normalize_requested_scopes(
        params
            .get("scope")
            .map(String::as_str)
            .unwrap_or("mcp:observe"),
    )?;
    let oauth_state = params.get("state").cloned().unwrap_or_default();
    if oauth_state.len() > 1024 {
        anyhow::bail!("state is too long");
    }
    let id = Uuid::new_v4().to_string();
    state.db.with_connection(|db| {
        db.execute(
            "INSERT INTO mcp_requests(id,user_id,client_id,redirect_uri,state,scope,code_challenge,resource,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?,?)",
            rusqlite::params![id,user.id,client_id,redirect,oauth_state,scopes.join(" "),challenge,resource,now_ms(),now_ms()+10*60_000],
        )?;
        Ok(())
    })?;
    Ok(serde_json::json!({
        "requestId":id,"clientName":client.0,"redirectUri":redirect,"requestedScopes":scopes
    }))
}

pub(super) fn normalize_requested_scopes(value: &str) -> anyhow::Result<Vec<String>> {
    let requested: Vec<_> = value.split_whitespace().collect();
    if requested.iter().any(|scope| !MCP_SCOPES.contains(scope)) {
        anyhow::bail!("unsupported MCP scope");
    }
    Ok(MCP_SCOPES
        .iter()
        .filter(|scope| requested.contains(scope))
        .map(|scope| (*scope).to_owned())
        .collect())
}

fn safe_redirect(value: &str) -> bool {
    let Ok(url) = url::Url::parse(value) else {
        return false;
    };
    if url.fragment().is_some() || !url.username().is_empty() || url.password().is_some() {
        return false;
    }
    match url.scheme() {
        "https" => true,
        "http" => matches!(url.host_str(), Some("localhost" | "127.0.0.1" | "::1")),
        "javascript" | "data" | "file" => false,
        scheme => {
            scheme
                .chars()
                .next()
                .is_some_and(|c| c.is_ascii_alphabetic())
                && scheme
                    .chars()
                    .all(|c| c.is_ascii_alphanumeric() || matches!(c, '+' | '-' | '.'))
        }
    }
}

fn client_row(state: &AppState, id: &str) -> anyhow::Result<Option<(String, String)>> {
    Ok(state.db.with_connection(|db| {
        db.query_row(
            "SELECT name,redirect_uris FROM mcp_clients WHERE id=?",
            [id],
            |row| Ok((row.get(0)?, row.get(1)?)),
        )
        .optional()
    })?)
}

pub(super) struct RequestRow {
    pub client_id: String,
    pub client_name: String,
    pub redirect_uri: String,
    pub state: String,
    pub scope: String,
    pub code_challenge: String,
    pub resource: String,
    pub prepared_grant: Option<String>,
}

pub(super) fn request_row(
    state: &AppState,
    user: &str,
    id: &str,
) -> anyhow::Result<Option<RequestRow>> {
    Ok(state.db.with_connection(|db| {
        db.query_row(
            "SELECT r.client_id,c.name,r.redirect_uri,r.state,r.scope,r.code_challenge,r.resource,r.prepared_grant FROM mcp_requests r JOIN mcp_clients c ON c.id=r.client_id WHERE r.id=? AND r.user_id=? AND r.expires_at>?",
            rusqlite::params![id,user,now_ms()],
            |row| Ok(RequestRow {
                client_id: row.get(0)?, client_name: row.get(1)?, redirect_uri: row.get(2)?,
                state: row.get(3)?, scope: row.get(4)?, code_challenge: row.get(5)?,
                resource: row.get(6)?, prepared_grant: row.get(7)?,
            }),
        ).optional()
    })?)
}
