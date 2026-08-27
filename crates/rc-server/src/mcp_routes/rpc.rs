use crate::{AppState, MCP_PROTOCOL_VERSION, access_grant};
use axum::{
    Json,
    body::Bytes,
    extract::State,
    http::{HeaderMap, StatusCode},
    response::{IntoResponse, Response},
};
use rc_protocol::McpGrantPayload;

pub(super) async fn mcp(
    State(state): State<AppState>,
    headers: HeaderMap,
    body: Bytes,
) -> Response {
    let parsed: serde_json::Value = match serde_json::from_slice(&body) {
        Ok(value) => value,
        Err(_) => {
            return rpc_error(
                serde_json::Value::Null,
                -32700,
                "Parse error",
                StatusCode::BAD_REQUEST,
            );
        }
    };
    let id = parsed.get("id").cloned().unwrap_or(serde_json::Value::Null);
    let method = parsed.get("method").and_then(serde_json::Value::as_str);
    if parsed.get("jsonrpc").and_then(|value| value.as_str()) != Some("2.0") || method.is_none() {
        return rpc_error(id, -32600, "Invalid Request", StatusCode::BAD_REQUEST);
    }
    let method = method.unwrap_or_default();
    let protocol = headers
        .get("mcp-protocol-version")
        .and_then(|value| value.to_str().ok())
        .unwrap_or("missing");
    if protocol != MCP_PROTOCOL_VERSION {
        return rpc_error(
            id,
            -32022,
            &format!("Unsupported protocol version: {protocol}"),
            StatusCode::BAD_REQUEST,
        );
    }
    if headers
        .get("mcp-method")
        .and_then(|value| value.to_str().ok())
        != Some(method)
    {
        return rpc_error(
            id,
            -32600,
            "Mcp-Method header does not match request",
            StatusCode::BAD_REQUEST,
        );
    }
    if method == "server/discover" {
        return rpc(
            id,
            serde_json::json!({"resultType":"complete","supportedVersions":[MCP_PROTOCOL_VERSION],"capabilities":{"tools":{}},"instructions":"Use only the machines and capabilities explicitly granted by the user.","ttlMs":300000,"cacheScope":"public","_meta":{"io.modelcontextprotocol/serverInfo":{"name":"RC","version":env!("CARGO_PKG_VERSION"),"websiteUrl":state.config.public_url.as_str()}}}),
        );
    }
    let token = headers
        .get("authorization")
        .and_then(|value| value.to_str().ok())
        .and_then(|value| value.strip_prefix("Bearer "))
        .unwrap_or_default();
    let grant = match access_grant(&state, token) {
        Ok(Some(grant)) => grant,
        _ => return auth_error(&state, "mcp:observe", false),
    };
    let payload: McpGrantPayload = match serde_json::from_str(&grant.grant) {
        Ok(value) => value,
        Err(_) => return auth_error(&state, "mcp:observe", false),
    };
    let context = crate::mcp_tools::McpContext {
        record: grant,
        payload,
    };
    match method {
        "tools/list" => rpc(
            id,
            serde_json::json!({"resultType":"complete","tools":crate::mcp_tools::tools_for(&context),"ttlMs":30000,"cacheScope":"private"}),
        ),
        "tools/call" => tool_call(&state, &headers, id, &parsed, &context).await,
        _ => rpc_error(id, -32601, "Method not found", StatusCode::OK),
    }
}

async fn tool_call(
    state: &AppState,
    headers: &HeaderMap,
    id: serde_json::Value,
    parsed: &serde_json::Value,
    context: &crate::mcp_tools::McpContext,
) -> Response {
    let name = parsed
        .pointer("/params/name")
        .and_then(|value| value.as_str())
        .unwrap_or_default();
    if headers
        .get("mcp-name")
        .and_then(|value| value.to_str().ok())
        != Some(name)
    {
        return rpc_error(
            id,
            -32600,
            "Mcp-Name header does not match tool call",
            StatusCode::BAD_REQUEST,
        );
    }
    let Some(scope) = crate::mcp_tools::registered_scope(name) else {
        return rpc_error(
            id,
            -32602,
            &format!("Tool is not available: {name}"),
            StatusCode::OK,
        );
    };
    if !crate::mcp_tools::has_scope(&context.payload.scopes, scope) {
        return auth_error(state, scope, true);
    }
    let args = parsed
        .pointer("/params/arguments")
        .and_then(|value| value.as_object())
        .cloned()
        .unwrap_or_default();
    match crate::mcp_tools::call_tool(state, context, name, &args).await {
        Ok(value) => rpc(id, value),
        Err(error) => rpc(
            id,
            serde_json::json!({"resultType":"complete","content":[{"type":"text","text":error.to_string()}],"isError":true}),
        ),
    }
}

fn rpc(id: serde_json::Value, result: serde_json::Value) -> Response {
    Json(serde_json::json!({"jsonrpc":"2.0","id":id,"result":result})).into_response()
}
fn rpc_error(id: serde_json::Value, code: i64, message: &str, status: StatusCode) -> Response {
    (
        status,
        Json(serde_json::json!({"jsonrpc":"2.0","id":id,"error":{"code":code,"message":message}})),
    )
        .into_response()
}
fn auth_error(state: &AppState, scope: &str, insufficient: bool) -> Response {
    let metadata = format!(
        "{}/.well-known/oauth-protected-resource",
        state.config.public_url.trim_end_matches('/')
    );
    let header = if insufficient {
        format!(
            "Bearer error=\"insufficient_scope\", resource_metadata=\"{metadata}\", scope=\"{scope}\""
        )
    } else {
        format!("Bearer resource_metadata=\"{metadata}\", scope=\"{scope}\"")
    };
    let status = if insufficient {
        StatusCode::FORBIDDEN
    } else {
        StatusCode::UNAUTHORIZED
    };
    (
        status,
        [("www-authenticate", header)],
        Json(
            serde_json::json!({"error":if insufficient{"insufficient_scope"}else{"unauthorized"}}),
        ),
    )
        .into_response()
}
