use super::{McpContext, complete, operation, running_owned_device};
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
    let device_id = args
        .get("deviceId")
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
    let device = running_owned_device(state, context, process_id, device_id)?;
    let (request_id, receiver) = state.mcp.status_request(&device)?;
    let message = ServerToNode::McpExecutionSignal {
        request_id: request_id.clone(),
        process_id: process_id.to_owned(),
        user_id: context.payload.user_id.clone(),
        signal: signal.clone(),
        mcp_grant: context.record.grant.clone(),
        mcp_signature: context.record.grant_signature.clone(),
        control_grant: context.record.control_grant.clone(),
        credential_id: context.record.credential_id.clone(),
        control_assertion: context.record.control_assertion.clone(),
    };
    operation::send(state, &device, &request_id, receiver, message).await?;
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
