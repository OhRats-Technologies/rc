use super::{McpContext, complete, running_owned_device};
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
    let signal = args
        .get("signal")
        .and_then(serde_json::Value::as_str)
        .unwrap_or("TERM")
        .to_ascii_uppercase();
    if !matches!(signal.as_str(), "INT" | "TERM" | "KILL") {
        anyhow::bail!("signal must be INT, TERM, or KILL");
    }
    let device = running_owned_device(state, context, process_id)?;
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
