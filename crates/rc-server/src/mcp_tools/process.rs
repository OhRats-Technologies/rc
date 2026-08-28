use super::{McpContext, complete, require_owned_device};
use crate::{AppState, McpProcessResult, now_ms};
use rc_protocol::ServerToNode;
use uuid::Uuid;

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
    let command = args
        .get("command")
        .and_then(serde_json::Value::as_str)
        .unwrap_or_default()
        .to_owned();
    if command.trim().is_empty() {
        anyhow::bail!("command must not be empty");
    }
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
    let process_id = Uuid::new_v4().to_string();
    state.mcp.register(
        &process_id,
        &context.payload.id,
        &context.payload.user_id,
        device,
    )?;
    if let Err(error) = insert_process(state, context, device, &process_id) {
        state.mcp.remove(&process_id);
        return Err(error);
    }
    let message = ServerToNode::McpStart {
        process_id: process_id.clone(),
        user_id: context.payload.user_id.clone(),
        command,
        cwd,
        mcp_grant: context.record.grant.clone(),
        mcp_signature: context.record.grant_signature.clone(),
        control_grant: context.record.control_grant.clone(),
        credential_id: context.record.credential_id.clone(),
        control_assertion: context.record.control_assertion.clone(),
    };
    if let Err(error) = state.nodes.send(device, &message).await {
        let reason = format!("RC command delivery failed: {error}");
        state.mcp.mark_lost(&process_id, &reason);
        state.lose_hosted_process(device, &process_id, &reason);
    }
    let result = state
        .mcp
        .result(
            &process_id,
            &context.payload.id,
            &context.payload.user_id,
            0,
            wait,
        )
        .await?;
    complete_result(result)
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
    let result = state
        .mcp
        .result(
            process_id,
            &context.payload.id,
            &context.payload.user_id,
            cursor,
            wait,
        )
        .await?;
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
        text.push_str(&chunk.text);
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
