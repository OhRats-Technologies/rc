use super::{McpContext, complete, running_owned_device};
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
    let data = args.get("data").and_then(serde_json::Value::as_str);
    let eof = args
        .get("eof")
        .and_then(serde_json::Value::as_bool)
        .unwrap_or(false);
    let (accepted_bytes, messages) = input_messages(process_id, data, eof)?;
    let device = running_owned_device(state, context, process_id)?;
    for message in messages {
        state.nodes.send(&device, &message).await?;
    }
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

fn input_messages(
    process_id: &str,
    data: Option<&str>,
    eof: bool,
) -> anyhow::Result<(usize, Vec<ServerToNode>)> {
    if process_id.is_empty() {
        anyhow::bail!("invalid process ID");
    }
    if data.is_none_or(str::is_empty) && !eof {
        anyhow::bail!("process_input requires data or eof=true");
    }
    let bytes = data.unwrap_or_default().as_bytes();
    if bytes.len() > PROCESS_INPUT_CHUNK_LIMIT {
        anyhow::bail!("process input exceeds the {PROCESS_INPUT_CHUNK_LIMIT}-byte transport chunk");
    }
    let mut messages = Vec::with_capacity(2);
    if !bytes.is_empty() {
        messages.push(ServerToNode::McpStdin {
            process_id: process_id.to_owned(),
            data: URL_SAFE_NO_PAD.encode(bytes),
        });
    }
    if eof {
        messages.push(ServerToNode::McpStdinClose {
            process_id: process_id.to_owned(),
        });
    }
    Ok((bytes.len(), messages))
}

#[cfg(test)]
mod tests {
    use super::input_messages;
    use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
    use rc_protocol::{PROCESS_INPUT_CHUNK_LIMIT, ServerToNode};

    #[test]
    fn input_is_exact_and_eof_is_a_distinct_followup() -> anyhow::Result<()> {
        let (accepted, messages) = input_messages("process", Some("42\n"), true)?;
        assert_eq!(accepted, 3);
        assert_eq!(messages.len(), 2);
        match &messages[0] {
            ServerToNode::McpStdin { process_id, data } => {
                assert_eq!(process_id, "process");
                assert_eq!(URL_SAFE_NO_PAD.decode(data)?, b"42\n");
            }
            other => anyhow::bail!("unexpected first input message: {other:?}"),
        }
        assert!(matches!(
            &messages[1],
            ServerToNode::McpStdinClose { process_id } if process_id == "process"
        ));
        Ok(())
    }

    #[test]
    fn empty_input_without_eof_is_rejected() {
        assert!(input_messages("process", Some(""), false).is_err());
        assert!(input_messages("process", None, false).is_err());
    }

    #[test]
    fn input_capacity_matches_the_decoded_transport_chunk() {
        let accepted = "x".repeat(PROCESS_INPUT_CHUNK_LIMIT);
        assert!(input_messages("process", Some(&accepted), false).is_ok());
        let rejected = "x".repeat(PROCESS_INPUT_CHUNK_LIMIT + 1);
        assert!(input_messages("process", Some(&rejected), false).is_err());
    }
}
