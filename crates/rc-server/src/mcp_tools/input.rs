use super::{McpContext, complete, operation, running_owned_device};
use crate::AppState;
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_protocol::{PROCESS_INPUT_CHUNK_LIMIT, ServerToNode};

pub(super) async fn input(
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
    let data = args.get("data").and_then(serde_json::Value::as_str);
    let encoding = args
        .get("encoding")
        .and_then(serde_json::Value::as_str)
        .unwrap_or("text");
    let eof = args
        .get("eof")
        .and_then(serde_json::Value::as_bool)
        .unwrap_or(false);
    let (accepted_bytes, data) = input_payload(process_id, data, encoding, eof)?;
    let device = running_owned_device(state, context, process_id, device_id)?;
    let (request_id, receiver) = state.mcp.status_request(&device)?;
    let message = ServerToNode::McpExecutionInput {
        request_id: request_id.clone(),
        process_id: process_id.to_owned(),
        user_id: context.payload.user_id.clone(),
        data,
        eof,
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
            "acceptedBytes": accepted_bytes,
            "stdinClosed": eof,
        }),
        format!(
            "Accepted {} byte{} for process {}{}.",
            accepted_bytes,
            if accepted_bytes == 1 { "" } else { "s" },
            process_id,
            if eof { " and closed stdin" } else { "" }
        ),
        false,
    ))
}

fn input_payload(
    process_id: &str,
    data: Option<&str>,
    encoding: &str,
    eof: bool,
) -> anyhow::Result<(usize, String)> {
    if process_id.is_empty() {
        anyhow::bail!("invalid process ID");
    }
    if data.is_none_or(str::is_empty) && !eof {
        anyhow::bail!("process_input requires data or eof=true");
    }
    let bytes = match encoding {
        "text" => data.unwrap_or_default().as_bytes().to_vec(),
        "base64" => base64::engine::general_purpose::STANDARD
            .decode(data.unwrap_or_default())
            .map_err(|_| anyhow::anyhow!("process input is not valid base64"))?,
        _ => anyhow::bail!("encoding must be text or base64"),
    };
    if bytes.len() > PROCESS_INPUT_CHUNK_LIMIT {
        anyhow::bail!("process input exceeds the {PROCESS_INPUT_CHUNK_LIMIT}-byte transport chunk");
    }
    Ok((bytes.len(), URL_SAFE_NO_PAD.encode(bytes)))
}

#[cfg(test)]
mod tests {
    use super::input_payload;
    use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
    use rc_protocol::PROCESS_INPUT_CHUNK_LIMIT;

    #[test]
    fn input_is_exact_and_eof_is_explicit() -> anyhow::Result<()> {
        let (accepted, data) = input_payload("process", Some("42\n"), "text", true)?;
        assert_eq!(accepted, 3);
        assert_eq!(URL_SAFE_NO_PAD.decode(data)?, b"42\n");
        Ok(())
    }

    #[test]
    fn empty_input_without_eof_is_rejected() {
        assert!(input_payload("process", Some(""), "text", false).is_err());
        assert!(input_payload("process", None, "text", false).is_err());
    }

    #[test]
    fn input_capacity_matches_the_decoded_transport_chunk() {
        let accepted = "x".repeat(PROCESS_INPUT_CHUNK_LIMIT);
        assert!(input_payload("process", Some(&accepted), "text", false).is_ok());
        let rejected = "x".repeat(PROCESS_INPUT_CHUNK_LIMIT + 1);
        assert!(input_payload("process", Some(&rejected), "text", false).is_err());
    }

    #[test]
    fn base64_input_preserves_arbitrary_bytes() -> anyhow::Result<()> {
        let (accepted, data) = input_payload("process", Some("/wCA"), "base64", false)?;
        assert_eq!(accepted, 3);
        assert_eq!(URL_SAFE_NO_PAD.decode(data)?, [0xff, 0x00, 0x80]);
        Ok(())
    }
}
