use super::{McpContext, require_owned_device};
use crate::AppState;
use base64::{Engine as _, engine::general_purpose::STANDARD};
use rc_protocol::ServerToNode;
use uuid::Uuid;

pub(super) async fn view(
    state: &AppState,
    context: &McpContext,
    args: &serde_json::Map<String, serde_json::Value>,
) -> anyhow::Result<serde_json::Value> {
    let device = args
        .get("deviceId")
        .and_then(serde_json::Value::as_str)
        .unwrap_or_default();
    require_owned_device(state, context, device)?;
    let path = args
        .get("path")
        .and_then(serde_json::Value::as_str)
        .unwrap_or_default();
    if path.trim().is_empty() {
        anyhow::bail!("path must not be empty");
    }
    let request_id = Uuid::new_v4().to_string();
    let message = ServerToNode::McpImageView {
        request_id: request_id.clone(),
        user_id: context.payload.user_id.clone(),
        path: path.to_owned(),
        mcp_grant: context.record.grant.clone(),
        mcp_signature: context.record.grant_signature.clone(),
        control_grant: context.record.control_grant.clone(),
        credential_id: context.record.credential_id.clone(),
        control_assertion: context.record.control_assertion.clone(),
    };
    let image = state
        .mcp
        .request_image(&state.nodes, device, &request_id, &message)
        .await?;
    let size_bytes = image.bytes.len();
    Ok(serde_json::json!({
        "resultType": "complete",
        "content": [
            {"type": "text", "text": format!("Image from {path} ({}, {size_bytes} bytes).", image.mime_type)},
            {"type": "image", "data": STANDARD.encode(&image.bytes), "mimeType": image.mime_type},
        ],
        "structuredContent": {
            "deviceId": device,
            "path": path,
            "mimeType": image.mime_type,
            "sizeBytes": size_bytes,
        },
    }))
}
