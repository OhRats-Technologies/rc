use crate::{AppState, MCP_PROTOCOL_VERSION, access_grant};
use axum::{
    Json,
    body::Bytes,
    extract::State,
    http::{HeaderMap, StatusCode},
    response::{IntoResponse, Response},
};
use rc_protocol::McpGrantPayload;
use tracing::Instrument as _;

const LEGACY_MCP_PROTOCOL_VERSION: &str = "2025-11-25";
const LEGACY_MCP_PROTOCOL_VERSIONS: [&str; 3] = ["2025-11-25", "2025-06-18", "2025-03-26"];

pub(super) async fn mcp(
    State(state): State<AppState>,
    headers: HeaderMap,
    body: Bytes,
) -> Response {
    let reference = uuid::Uuid::new_v4().to_string();
    let (method, tool) = super::diagnostics::labels(&body);
    let span = tracing::info_span!("mcp_request", request_id = %reference, method, tool);
    async {
        let started = std::time::Instant::now();
        tracing::info!("MCP request received");
        let mut response = handle(state, headers, body, &reference).await;
        tracing::info!(
            http_status = response.status().as_u16(),
            elapsed_ms = started.elapsed().as_millis() as u64,
            "MCP request completed"
        );
        response
            .headers_mut()
            .insert("x-rc-request-id", reference.parse().expect("UUID header"));
        response
    }
    .instrument(span)
    .await
}

async fn handle(state: AppState, headers: HeaderMap, body: Bytes, reference: &str) -> Response {
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

    if method == "initialize" {
        return legacy_initialize(id, &parsed);
    }

    let protocol = headers
        .get("mcp-protocol-version")
        .and_then(|value| value.to_str().ok())
        .unwrap_or("missing");
    let modern = protocol == MCP_PROTOCOL_VERSION;
    let legacy = protocol == "missing" || LEGACY_MCP_PROTOCOL_VERSIONS.contains(&protocol);

    if method == "notifications/initialized" && legacy {
        return StatusCode::ACCEPTED.into_response();
    }
    if !modern && !legacy {
        tracing::warn!(
            protocol_version = protocol_label(protocol),
            "MCP protocol version rejected"
        );
        return rpc_error(
            id,
            -32022,
            &format!("Unsupported protocol version: {protocol}"),
            StatusCode::BAD_REQUEST,
        );
    }
    if modern
        && headers
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
        if !modern {
            return rpc_error(id, -32601, "Method not found", StatusCode::OK);
        }
        return rpc(
            id,
            serde_json::json!({"resultType":"complete","supportedVersions":[MCP_PROTOCOL_VERSION],"capabilities":{"tools":{}} ,"instructions":"Use only the machines and capabilities explicitly granted by the user.","ttlMs":300000,"cacheScope":"public","_meta":{"io.modelcontextprotocol/serverInfo":{"name":"RC","version":env!("CARGO_PKG_VERSION"),"websiteUrl":state.config.public_url.as_str()}}}),
        );
    }

    let token = headers
        .get("authorization")
        .and_then(|value| value.to_str().ok())
        .and_then(|value| value.strip_prefix("Bearer "))
        .unwrap_or_default();
    let grant = match access_grant(&state, token) {
        Ok(Some(grant)) => grant,
        _ => return auth_error(&state, "mcp:observe", false, reference),
    };
    let payload: McpGrantPayload = match serde_json::from_str(&grant.grant) {
        Ok(value) => value,
        Err(_) => return auth_error(&state, "mcp:observe", false, reference),
    };
    let context = crate::mcp_tools::McpContext {
        record: grant,
        payload,
    };
    match method {
        "tools/list" => rpc(id, tools_result(&context, modern)),
        "tools/call" => tool_call(&state, &headers, id, &parsed, &context, reference, modern).await,
        _ => rpc_error(id, -32601, "Method not found", StatusCode::OK),
    }
}

fn legacy_initialize(id: serde_json::Value, parsed: &serde_json::Value) -> Response {
    let Some(requested) = parsed
        .pointer("/params/protocolVersion")
        .and_then(serde_json::Value::as_str)
    else {
        return rpc_error(id, -32602, "Invalid params", StatusCode::OK);
    };
    let protocol = if LEGACY_MCP_PROTOCOL_VERSIONS.contains(&requested) {
        requested
    } else {
        LEGACY_MCP_PROTOCOL_VERSION
    };
    rpc(
        id,
        serde_json::json!({
            "protocolVersion": protocol,
            "capabilities": {"tools": {}},
            "serverInfo": {
                "name": "RC",
                "version": env!("CARGO_PKG_VERSION")
            },
            "instructions": "Use only the machines and capabilities explicitly granted by the user."
        }),
    )
}

fn tools_result(context: &crate::mcp_tools::McpContext, modern: bool) -> serde_json::Value {
    let tools = crate::mcp_tools::tools_for(context);
    if modern {
        serde_json::json!({
            "resultType": "complete",
            "tools": tools,
            "ttlMs": 30000,
            "cacheScope": "private"
        })
    } else {
        serde_json::json!({"tools": tools})
    }
}

async fn tool_call(
    state: &AppState,
    headers: &HeaderMap,
    id: serde_json::Value,
    parsed: &serde_json::Value,
    context: &crate::mcp_tools::McpContext,
    reference: &str,
    modern: bool,
) -> Response {
    let name = parsed
        .pointer("/params/name")
        .and_then(|value| value.as_str())
        .unwrap_or_default();
    if modern
        && headers
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
        return auth_error(state, scope, true, reference);
    }
    let args = parsed
        .pointer("/params/arguments")
        .and_then(|value| value.as_object())
        .cloned()
        .unwrap_or_default();
    let result = crate::mcp_tools::call_tool(state, context, name, &args).await;
    let mut result = super::diagnostics::tool_result(result, reference);
    if !modern {
        result
            .as_object_mut()
            .map(|value| value.remove("resultType"));
    }
    rpc(id, result)
}

fn protocol_label(protocol: &str) -> &'static str {
    match protocol {
        "2026-07-28" => "2026-07-28",
        "2025-11-25" => "2025-11-25",
        "2025-06-18" => "2025-06-18",
        "2025-03-26" => "2025-03-26",
        "2024-11-05" => "2024-11-05",
        "missing" => "missing",
        _ => "other",
    }
}

fn rpc(id: serde_json::Value, result: serde_json::Value) -> Response {
    Json(serde_json::json!({"jsonrpc":"2.0","id":id,"result":result})).into_response()
}

fn rpc_error(id: serde_json::Value, code: i64, message: &str, status: StatusCode) -> Response {
    tracing::warn!(rpc_code = code, "MCP protocol request rejected");
    (
        status,
        Json(serde_json::json!({"jsonrpc":"2.0","id":id,"error":{"code":code,"message":message}})),
    )
        .into_response()
}

fn auth_error(state: &AppState, scope: &str, insufficient: bool, reference: &str) -> Response {
    tracing::warn!(
        error_code = if insufficient {
            "insufficient_scope"
        } else {
            "unauthorized"
        },
        "MCP authorization rejected"
    );
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
            serde_json::json!({"error":if insufficient{"insufficient_scope"}else{"unauthorized"},"requestId":reference}),
        ),
    )
        .into_response()
}
