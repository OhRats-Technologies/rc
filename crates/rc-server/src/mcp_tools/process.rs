use super::{McpContext, complete, require_owned_device};
use crate::{AppState, McpProcessResult, now_ms};
use rc_protocol::ServerToNode;
use uuid::Uuid;

mod node_status;
mod request;
use node_status::{lost_result, query_node_status};
use request::{environment, execution_mode};

pub(super) async fn run(
    state: &AppState,
    context: &McpContext,
    args: &serde_json::Map<String, serde_json::Value>,
) -> anyhow::Result<serde_json::Value> {
    let device = args
        .get("deviceId")
        .and_then(serde_json::Value::as_str)
        .unwrap_or_default();
    require_owned_device(state, context, device)?;
    require_execution_v2(state, device)?;
    let mode = execution_mode(args)?;
    let environment = environment(args)?;
    let cwd = args
        .get("cwd")
        .and_then(serde_json::Value::as_str)
        .unwrap_or_default()
        .to_owned();
    let wait = args
        .get("waitSeconds")
        .or_else(|| args.get("timeoutSeconds"))
        .and_then(serde_json::Value::as_u64)
        .unwrap_or(20)
        .min(60);
    let max_runtime_seconds = args
        .get("maxRuntimeSeconds")
        .and_then(serde_json::Value::as_u64);
    let process_id = Uuid::new_v4().to_string();
    insert_process(state, context, device, &process_id)?;
    let message = ServerToNode::McpStart {
        process_id: process_id.clone(),
        user_id: context.payload.user_id.clone(),
        mode,
        cwd,
        environment,
        max_runtime_seconds,
        mcp_grant: context.record.grant.clone(),
        mcp_signature: context.record.grant_signature.clone(),
        control_grant: context.record.control_grant.clone(),
        credential_id: context.record.credential_id.clone(),
        control_assertion: context.record.control_assertion.clone(),
    };
    if let Err(error) = state.nodes.send(device, &message).await {
        let reason = format!("RC command delivery failed: {error}");
        state.lose_hosted_process(device, &process_id, &reason);
        return complete_result(lost_result(process_id, reason));
    }
    let result = query_node_status(state, context, device, &process_id, 0, wait).await?;
    complete_result(result)
}

fn require_execution_v2(state: &AppState, device: &str) -> anyhow::Result<()> {
    let capabilities = state.db.with_connection(|db| {
        db.query_row(
            "SELECT capabilities FROM devices WHERE id=?",
            [device],
            |row| row.get::<_, String>(0),
        )
    })?;
    let capabilities = serde_json::from_str::<Vec<String>>(&capabilities).unwrap_or_default();
    if !capabilities.iter().any(|value| value == "execution-v2") {
        anyhow::bail!("Node upgrade required: execution-v2 is unavailable");
    }
    Ok(())
}

pub(super) async fn status(
    state: &AppState,
    context: &McpContext,
    args: &serde_json::Map<String, serde_json::Value>,
) -> anyhow::Result<serde_json::Value> {
    let process_id = args
        .get("processId")
        .and_then(serde_json::Value::as_str)
        .unwrap_or_default();
    let device = args
        .get("deviceId")
        .and_then(serde_json::Value::as_str)
        .unwrap_or_default();
    if process_id.is_empty() {
        anyhow::bail!("invalid process ID");
    }
    let cursor = args
        .get("cursor")
        .or_else(|| args.get("offset"))
        .and_then(serde_json::Value::as_u64)
        .unwrap_or(0) as usize;
    let wait = args
        .get("waitSeconds")
        .and_then(serde_json::Value::as_u64)
        .unwrap_or(0)
        .min(60);
    require_owned_device(state, context, device)?;
    let result = query_node_status(state, context, device, process_id, cursor, wait).await?;
    complete_result(result)
}

fn insert_process(
    state: &AppState,
    context: &McpContext,
    device: &str,
    process_id: &str,
) -> anyhow::Result<()> {
    state.db.with_connection(|db| {
        db.execute(
            "INSERT INTO processes(id,device_id,origin,status,terminal,created_by,created_at) \
             VALUES(?,?,'mcp','starting',0,?,?)",
            rusqlite::params![process_id, device, context.payload.user_id, now_ms()],
        )?;
        Ok(())
    })?;
    Ok(())
}

fn complete_result(result: McpProcessResult) -> anyhow::Result<serde_json::Value> {
    let mut text = String::new();
    for chunk in &result.chunks {
        if chunk.stream == "stderr" {
            text.push_str("[stderr] ");
        }
        if chunk.encoding == "text" {
            text.push_str(&chunk.data);
        } else {
            text.push_str(&format!("[base64 output: {}]", chunk.data));
        }
    }
    if result.truncated_before_cursor > 0 {
        text.push_str(&format!(
            "\n[Earlier output before cursor {} is no longer buffered.]",
            result.truncated_before_cursor
        ));
    }
    let status = match result.status.as_str() {
        "exited" => format!(
            "Exit {}.",
            result
                .exit_code
                .map(|value| value.to_string())
                .unwrap_or_else(|| "unknown".into())
        ),
        "running" => format!("Process {} is still running.", result.process_id),
        _ => format!(
            "Process was lost{}",
            result
                .error
                .as_ref()
                .map(|error| format!(": {error}"))
                .unwrap_or_else(|| ".".into())
        ),
    };
    if !text.is_empty() && !text.ends_with('\n') {
        text.push('\n');
    }
    text.push_str(&status);
    if result.output_pending {
        text.push_str(" More buffered output is available at nextCursor.");
    }
    Ok(complete(serde_json::to_value(&result)?, text, false))
}
