use super::{McpContext, McpProcessResult};
use crate::{AppState, McpOutputChunk};
use base64::Engine as _;
use rc_protocol::{NodeToServer, ServerToNode};

pub(super) async fn query_node_status(
    state: &AppState,
    context: &McpContext,
    device: &str,
    process_id: &str,
    cursor: usize,
    wait: u64,
) -> anyhow::Result<McpProcessResult> {
    let (request_id, receiver) = state.mcp.status_request(device)?;
    let send = state
        .nodes
        .send(
            device,
            &ServerToNode::McpExecutionStatus {
                request_id: request_id.clone(),
                process_id: process_id.to_owned(),
                user_id: context.payload.user_id.clone(),
                cursor: u64::try_from(cursor).unwrap_or(u64::MAX),
                wait_seconds: wait,
                mcp_grant: context.record.grant.clone(),
                mcp_signature: context.record.grant_signature.clone(),
                control_grant: context.record.control_grant.clone(),
                credential_id: context.record.credential_id.clone(),
                control_assertion: context.record.control_assertion.clone(),
            },
        )
        .await;
    if let Err(error) = send {
        state.mcp.cancel_status_request(&request_id);
        return Err(error);
    }
    let message = match tokio::time::timeout(
        std::time::Duration::from_secs(wait.min(60) + 5),
        receiver,
    )
    .await
    {
        Ok(result) => result?,
        Err(_) => {
            state.mcp.cancel_status_request(&request_id);
            anyhow::bail!("Node process status timed out");
        }
    };
    convert(message)
}

fn convert(message: NodeToServer) -> anyhow::Result<McpProcessResult> {
    let NodeToServer::McpExecutionStatusResult {
        process_id,
        status,
        chunks,
        next_cursor,
        truncated_before_cursor,
        output_pending,
        exit_code,
        signal,
        error,
        ..
    } = message
    else {
        anyhow::bail!("invalid Node process status response");
    };
    Ok(McpProcessResult {
        process_id,
        status,
        chunks: chunks
            .into_iter()
            .map(|chunk| {
                let bytes = base64::engine::general_purpose::URL_SAFE_NO_PAD
                    .decode(chunk.data)
                    .map_err(|_| anyhow::anyhow!("invalid Node process output encoding"))?;
                let stream = match chunk.stream.as_str() {
                    "stdout" => "stdout",
                    "stderr" => "stderr",
                    _ => anyhow::bail!("invalid Node process output stream"),
                };
                let (encoding, data) = match String::from_utf8(bytes) {
                    Ok(text) => ("text", text),
                    Err(error) => (
                        "base64",
                        base64::engine::general_purpose::STANDARD.encode(error.into_bytes()),
                    ),
                };
                Ok(McpOutputChunk {
                    stream,
                    cursor: usize::try_from(chunk.cursor).unwrap_or(usize::MAX),
                    encoding,
                    data,
                })
            })
            .collect::<anyhow::Result<_>>()?,
        exit_code,
        signal: (!signal.is_empty()).then_some(signal),
        error: (!error.is_empty()).then_some(error),
        next_cursor: usize::try_from(next_cursor).unwrap_or(usize::MAX),
        output_pending,
        truncated_before_cursor: usize::try_from(truncated_before_cursor).unwrap_or(usize::MAX),
    })
}

pub(super) fn lost_result(process_id: String, error: String) -> McpProcessResult {
    McpProcessResult {
        process_id,
        status: "lost".into(),
        chunks: Vec::new(),
        exit_code: None,
        signal: None,
        error: Some(error),
        next_cursor: 0,
        output_pending: false,
        truncated_before_cursor: 0,
    }
}

#[cfg(test)]
mod tests {
    use super::convert;
    use rc_protocol::{McpExecutionChunk, NodeToServer};

    fn response(stream: &str, data: &str) -> NodeToServer {
        NodeToServer::McpExecutionStatusResult {
            request_id: "request".into(),
            process_id: "process".into(),
            status: "running".into(),
            chunks: vec![McpExecutionChunk {
                stream: stream.into(),
                cursor: 0,
                data: data.into(),
            }],
            next_cursor: 1,
            truncated_before_cursor: 0,
            output_pending: false,
            exit_code: None,
            signal: String::new(),
            error: String::new(),
        }
    }

    #[test]
    fn malformed_node_output_is_rejected_instead_of_silently_dropped() {
        assert!(convert(response("stdout", "%%%")).is_err());
        assert!(convert(response("unknown", "YQ")).is_err());
    }
}
