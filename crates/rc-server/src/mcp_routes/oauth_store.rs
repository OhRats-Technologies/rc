use crate::AppState;
use crate::auth_public_routes::ApiError;
use rc_protocol::McpGrantPayload;
use rusqlite::OptionalExtension;

pub(super) fn request_redirect(
    state: &AppState,
    user_id: &str,
    request_id: &str,
) -> Result<(String, String), ApiError> {
    state
        .db
        .with_connection(|db| {
            db.query_row(
                "SELECT redirect_uri,state FROM mcp_requests WHERE id=? AND user_id=? AND expires_at>?",
                rusqlite::params![request_id, user_id, crate::now_ms()],
                |row| Ok((row.get(0)?, row.get(1)?)),
            )
            .optional()
        })?
        .ok_or_else(|| ApiError::gone("MCP authorization expired"))
}

pub(super) struct RequestFull {
    client_id: String,
    redirect_uri: String,
    state: String,
    scope: String,
    code_challenge: String,
    resource: String,
}

pub(super) fn request_full(
    state: &AppState,
    user_id: &str,
    request_id: &str,
) -> Result<RequestFull, ApiError> {
    state
        .db
        .with_connection(|db| {
            db.query_row(
                "SELECT client_id,redirect_uri,state,scope,code_challenge,resource FROM mcp_requests WHERE id=? AND user_id=? AND expires_at>?",
                rusqlite::params![request_id, user_id, crate::now_ms()],
                |row| {
                    Ok(RequestFull {
                        client_id: row.get(0)?,
                        redirect_uri: row.get(1)?,
                        state: row.get(2)?,
                        scope: row.get(3)?,
                        code_challenge: row.get(4)?,
                        resource: row.get(5)?,
                    })
                },
            )
            .optional()
        })?
        .ok_or_else(|| ApiError::gone("MCP authorization expired"))
}

pub(super) fn oauth_query(request: &RequestFull) -> String {
    let mut serializer = url::form_urlencoded::Serializer::new(String::from("/oauth/authorize?"));
    serializer
        .append_pair("response_type", "code")
        .append_pair("client_id", &request.client_id)
        .append_pair("redirect_uri", &request.redirect_uri)
        .append_pair("scope", &request.scope)
        .append_pair("state", &request.state)
        .append_pair("code_challenge", &request.code_challenge)
        .append_pair("code_challenge_method", "S256")
        .append_pair("resource", &request.resource);
    serializer.finish()
}

pub(super) fn grant_workspaces(state: &AppState, grant_id: &str) -> Result<Vec<String>, ApiError> {
    let grant: Option<String> = state.db.with_connection(|db| {
        db.query_row(
            "SELECT grant FROM mcp_grants WHERE id=?",
            [grant_id],
            |row| row.get(0),
        )
        .optional()
    })?;
    let Some(grant) = grant else {
        return Ok(Vec::new());
    };
    let payload: McpGrantPayload =
        serde_json::from_str(&grant).map_err(|_| ApiError::bad_request("invalid MCP grant"))?;
    let mut workspaces = Vec::new();
    for device_id in payload.device_ids {
        let workspace = state.db.with_connection(|db| {
            db.query_row(
                "SELECT workspace_id FROM devices WHERE id=?",
                [device_id],
                |row| row.get::<_, String>(0),
            )
            .optional()
        })?;
        if let Some(workspace) = workspace
            && !workspaces.contains(&workspace)
        {
            workspaces.push(workspace);
        }
    }
    workspaces.sort();
    Ok(workspaces)
}
