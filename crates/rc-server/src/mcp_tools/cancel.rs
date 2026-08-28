use super::{McpContext, complete};
use crate::AppState;
use rc_protocol::ServerToNode;

pub(super) async fn cancel(
    state: &AppState,
    context: &McpContext,
    args: &serde_json::Map<String, serde_json::Value>,
) -> anyhow::Result<serde_json::Value> {
    let process_id = args
        .get("processId")
        .and_then(serde_json::Value::as_str)
        .unwrap_or_default();
    if process_id.is_empty() || process_id.len() > 100 {
        anyhow::bail!("invalid process ID");
    }
    let signal = args
        .get("signal")
        .and_then(serde_json::Value::as_str)
        .unwrap_or("TERM")
        .to_ascii_uppercase();
    if !matches!(signal.as_str(), "INT" | "TERM" | "KILL") {
        anyhow::bail!("signal must be INT, TERM, or KILL");
    }
    let device =
        state
            .mcp
            .running_device(process_id, &context.payload.id, &context.payload.user_id)?;
    let role = state
        .db
        .device_role(&context.payload.user_id, &device)?
        .ok_or_else(|| anyhow::anyhow!("Owner access is no longer available for this device"))?;
    if role != "owner" {
        anyhow::bail!("Owner access is no longer available for this device");
    }
    state
        .nodes
        .send(
            &device,
            &ServerToNode::McpSignal {
                process_id: process_id.to_owned(),
                signal: signal.clone(),
            },
        )
        .await
        .map_err(|_| anyhow::anyhow!("RC Node is offline"))?;
    Ok(complete(
        serde_json::json!({
            "processId": process_id,
            "signal": signal,
            "accepted": true,
        }),
        format!("Requested {signal} for process {process_id}."),
        false,
    ))
}
