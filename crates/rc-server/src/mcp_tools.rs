mod cancel;
mod descriptors;

use crate::{AppState, McpGrantRecord, McpProcessResult, now_ms};
use descriptors::{cancel_descriptor, machines_descriptor, run_descriptor, status_descriptor};
use rc_protocol::{McpGrantPayload, ServerToNode};
use uuid::Uuid;

#[derive(Clone)]
pub struct McpContext {
    pub record: McpGrantRecord,
    pub payload: McpGrantPayload,
}

pub fn tools_for(context: &McpContext) -> Vec<serde_json::Value> {
    let mut tools = vec![machines_descriptor(), status_descriptor()];
    if has_scope(&context.payload.scopes, "mcp:terminal") {
        tools.insert(1, run_descriptor());
        tools.insert(2, cancel_descriptor());
    }
    tools
}

pub fn registered_scope(name: &str) -> Option<&'static str> {
    match name {
        "machines_list" | "process_status" => Some("mcp:observe"),
        "process_run" | "process_cancel" => Some("mcp:terminal"),
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
        "process_run" => process_run(state, context, args).await,
        "process_cancel" => cancel::cancel(state, context, args).await,
        "process_status" => process_status(state, context, args).await,
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
            .map(|m| {
                format!(
                    "{} — {} — {}/{} — workspace {} — node {} — id {}",
                    m["name"].as_str().unwrap_or("machine"),
                    if m["online"].as_bool().unwrap_or(false) {
                        "online"
                    } else {
                        "offline"
                    },
                    m["platform"].as_str().unwrap_or("unknown"),
                    m["arch"].as_str().unwrap_or("unknown"),
                    m["workspace"].as_str().unwrap_or("unknown"),
                    m["nodeVersion"].as_str().unwrap_or("unknown"),
                    m["id"].as_str().unwrap_or("unknown")
                )
            })
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

async fn process_run(
    state: &AppState,
    context: &McpContext,
    args: &serde_json::Map<String, serde_json::Value>,
) -> anyhow::Result<serde_json::Value> {
    let device = args
        .get("deviceId")
        .and_then(|v| v.as_str())
        .unwrap_or_default();
    if !context.payload.device_ids.iter().any(|id| id == device) {
        anyhow::bail!("device is outside this MCP grant");
    }
    let role = state
        .db
        .device_role(&context.payload.user_id, device)?
        .ok_or_else(|| anyhow::anyhow!("operator access is no longer available for this device"))?;
    if role != "owner" {
        anyhow::bail!("Owner access is no longer available for this device");
    }
    let command = args
        .get("command")
        .and_then(|v| v.as_str())
        .unwrap_or_default()
        .trim()
        .to_owned();
    if command.is_empty() || command.len() > 8192 {
        anyhow::bail!("invalid command");
    }
    let cwd = args
        .get("cwd")
        .and_then(|v| v.as_str())
        .unwrap_or_default()
        .trim()
        .chars()
        .take(4096)
        .collect::<String>();
    let timeout = args
        .get("timeoutSeconds")
        .and_then(|v| v.as_u64())
        .unwrap_or(20)
        .clamp(1, 60);
    let process_id = Uuid::new_v4().to_string();
    state.db.with_connection(|db| {
        db.execute(
            "INSERT INTO processes(id,device_id,origin,status,terminal,created_by,created_at) \
             VALUES(?,?,'mcp','starting',0,?,?)",
            rusqlite::params![process_id, device, context.payload.user_id, now_ms()],
        )?;
        Ok(())
    })?;
    state.mcp.register(
        &process_id,
        &context.payload.id,
        &context.payload.user_id,
        device,
    );
    let sent = state
        .nodes
        .send(
            device,
            &ServerToNode::McpStart {
                process_id: process_id.clone(),
                user_id: context.payload.user_id.clone(),
                command,
                cwd,
                mcp_grant: context.record.grant.clone(),
                mcp_signature: context.record.grant_signature.clone(),
                control_grant: context.record.control_grant.clone(),
                credential_id: context.record.credential_id.clone(),
                control_assertion: context.record.control_assertion.clone(),
            },
        )
        .await;
    if sent.is_err() {
        state.mcp.mark_lost(&process_id, "RC Node is offline");
        state.lose_hosted_process(device, &process_id, "RC Node is offline");
    }
    let result = state
        .mcp
        .result(
            &process_id,
            &context.payload.id,
            &context.payload.user_id,
            0,
            timeout,
        )
        .await?;
    complete_result(result)
}

async fn process_status(
    state: &AppState,
    context: &McpContext,
    args: &serde_json::Map<String, serde_json::Value>,
) -> anyhow::Result<serde_json::Value> {
    let process = args
        .get("processId")
        .and_then(|v| v.as_str())
        .unwrap_or_default();
    let offset = args.get("offset").and_then(|v| v.as_u64()).unwrap_or(0) as usize;
    let wait = args
        .get("waitSeconds")
        .and_then(|v| v.as_u64())
        .unwrap_or(0)
        .min(60);
    let result = state
        .mcp
        .result(
            process,
            &context.payload.id,
            &context.payload.user_id,
            offset,
            wait,
        )
        .await?;
    complete_result(result)
}

fn complete_result(result: McpProcessResult) -> anyhow::Result<serde_json::Value> {
    let status = match result.status.as_str() {
        "exited" => format!(
            "Exit {}.",
            result
                .exit_code
                .map(|v| v.to_string())
                .unwrap_or_else(|| "unknown".into())
        ),
        "running" => format!("Process {} is still running.", result.process_id),
        _ => format!(
            "Process was lost{}",
            result
                .error
                .as_ref()
                .map(|e| format!(": {e}"))
                .unwrap_or_else(|| ".".into())
        ),
    };
    let suffix = if result.output_truncated {
        format!("{status} Output buffer is truncated.")
    } else {
        status
    };
    let text = if result.output.trim().is_empty() {
        suffix
    } else {
        format!("{}\n{suffix}", result.output.trim_end())
    };
    Ok(complete(serde_json::to_value(&result)?, text, false))
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
