mod cancel;
mod descriptors;
mod image;
mod input;
mod process;

use crate::{AppState, McpGrantRecord};
use descriptors::{
    cancel_descriptor, image_descriptor, input_descriptor, machines_descriptor, run_descriptor,
    status_descriptor,
};
use rc_protocol::McpGrantPayload;

#[derive(Clone)]
pub struct McpContext {
    pub record: McpGrantRecord,
    pub payload: McpGrantPayload,
}

pub fn tools_for(context: &McpContext) -> Vec<serde_json::Value> {
    let mut tools = vec![machines_descriptor(), status_descriptor()];
    if has_scope(&context.payload.scopes, "mcp:terminal") {
        tools = vec![
            machines_descriptor(),
            image_descriptor(),
            run_descriptor(),
            status_descriptor(),
            input_descriptor(),
            cancel_descriptor(),
        ];
    }
    tools
}

pub fn registered_scope(name: &str) -> Option<&'static str> {
    match name {
        "machines_list" | "process_status" => Some("mcp:observe"),
        "image_view" | "process_run" | "process_input" | "process_cancel" => Some("mcp:terminal"),
        _ => None,
    }
}

pub async fn call_tool(
    state: &AppState,
    context: &McpContext,
    name: &str,
    args: &serde_json::Map<String, serde_json::Value>,
) -> anyhow::Result<serde_json::Value> {
    match name {
        "machines_list" => machines(state, context).await,
        "image_view" => image::view(state, context, args).await,
        "process_run" => process::run(state, context, args).await,
        "process_status" => process::status(state, context, args).await,
        "process_input" => input::input(state, context, args).await,
        "process_cancel" => cancel::cancel(state, context, args).await,
        _ => anyhow::bail!("Tool is not available: {name}"),
    }
}

pub fn has_scope(scopes: &[String], required: &str) -> bool {
    if required == "mcp:observe" {
        scopes.iter().any(|scope| scope.starts_with("mcp:"))
    } else {
        scopes.iter().any(|scope| scope == required)
    }
}

async fn machines(state: &AppState, context: &McpContext) -> anyhow::Result<serde_json::Value> {
    let all = crate::devices_json(state, &context.payload.user_id).await?;
    let allowed = &context.payload.device_ids;
    let machines: Vec<_> = all
        .into_iter()
        .filter(|device| {
            device
                .get("id")
                .and_then(serde_json::Value::as_str)
                .is_some_and(|id| allowed.iter().any(|allowed_id| allowed_id == id))
        })
        .map(machine_view)
        .collect();
    let text = if machines.is_empty() {
        "No machines are available in this grant.".into()
    } else {
        machines
            .iter()
            .map(machine_text)
            .collect::<Vec<_>>()
            .join("\n")
    };
    Ok(complete(
        serde_json::json!({"machines": machines}),
        text,
        false,
    ))
}

fn machine_view(device: serde_json::Value) -> serde_json::Value {
    serde_json::json!({
        "id": device["id"],
        "name": device["name"],
        "workspaceId": device["workspace_id"],
        "workspace": device["workspace_name"],
        "hostname": device["hostname"],
        "platform": device["platform"],
        "arch": device["arch"],
        "nodeVersion": device["version"],
        "online": device["online"],
        "activeProcesses": device["active_processes"],
    })
}

fn machine_text(machine: &serde_json::Value) -> String {
    format!(
        "{} — {} — {}/{} — workspace {} — node {} — id {}",
        machine["name"].as_str().unwrap_or("machine"),
        if machine["online"].as_bool().unwrap_or(false) {
            "online"
        } else {
            "offline"
        },
        machine["platform"].as_str().unwrap_or("unknown"),
        machine["arch"].as_str().unwrap_or("unknown"),
        machine["workspace"].as_str().unwrap_or("unknown"),
        machine["nodeVersion"].as_str().unwrap_or("unknown"),
        machine["id"].as_str().unwrap_or("unknown")
    )
}

pub(super) fn require_owned_device(
    state: &AppState,
    context: &McpContext,
    device_id: &str,
) -> anyhow::Result<()> {
    if device_id.is_empty() || !context.payload.device_ids.iter().any(|id| id == device_id) {
        anyhow::bail!("device is outside this MCP grant");
    }
    let role = state
        .db
        .device_role(&context.payload.user_id, device_id)?
        .ok_or_else(|| anyhow::anyhow!("Owner access is no longer available for this device"))?;
    if role != "owner" {
        anyhow::bail!("Owner access is no longer available for this device");
    }
    Ok(())
}

pub(super) fn running_owned_device(
    state: &AppState,
    context: &McpContext,
    process_id: &str,
) -> anyhow::Result<String> {
    if process_id.is_empty() {
        anyhow::bail!("invalid process ID");
    }
    let device =
        state
            .mcp
            .running_device(process_id, &context.payload.id, &context.payload.user_id)?;
    require_owned_device(state, context, &device)?;
    Ok(device)
}

pub(super) fn complete(
    value: serde_json::Value,
    text: String,
    is_error: bool,
) -> serde_json::Value {
    let mut out = serde_json::json!({
        "resultType": "complete",
        "content": [{"type": "text", "text": text}],
        "structuredContent": value,
    });
    if is_error {
        out["isError"] = serde_json::Value::Bool(true);
    }
    out
}
