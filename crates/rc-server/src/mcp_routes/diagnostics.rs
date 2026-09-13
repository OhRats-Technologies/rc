use serde_json::{Value, json};

// Only fixed labels enter logs. Never log request arguments, RPC IDs, errors,
// command/output text, OAuth headers, or Node-supplied diagnostic strings.
pub(super) fn labels(body: &[u8]) -> (&'static str, &'static str) {
    let value: Value = serde_json::from_slice(body).unwrap_or(Value::Null);
    let method = match value["method"].as_str() {
        Some("server/discover") => "server/discover",
        Some("initialize") => "initialize",
        Some("notifications/initialized") => "notifications/initialized",
        Some("tools/list") => "tools/list",
        Some("tools/call") => "tools/call",
        _ => "unknown",
    };
    let tool = match value.pointer("/params/name").and_then(Value::as_str) {
        Some("machines_list") => "machines_list",
        Some("process_run") => "process_run",
        Some("process_status") => "process_status",
        Some("process_input") => "process_input",
        Some("process_cancel") => "process_cancel",
        Some("image_view") => "image_view",
        _ => "unknown",
    };
    (method, tool)
}

fn failure(message: &str) -> (&'static str, &'static str) {
    if message.starts_with("deviceId is required.") {
        (
            "missing_device_id",
            "Refresh the connector's tool definitions and supply the deviceId from machines_list.",
        )
    } else if message == "device is outside this MCP grant"
        || message == "Owner access is no longer available for this device"
    {
        (
            "grant_access_denied",
            "Check the selected device and reconnect the MCP grant if its permissions changed.",
        )
    } else if message.starts_with("Node is offline or still connecting;") {
        (
            "node_offline",
            "The command was not sent. Check machine presence before trying again.",
        )
    } else if message.starts_with("Node upgrade required:") {
        (
            "node_upgrade_required",
            "Upgrade the Node before retrying this operation.",
        )
    } else if message == "process is unavailable" {
        (
            "node_process_unavailable",
            "The Node could not authorize or read this process; older Nodes do not distinguish these causes. Do not automatically replay the command.",
        )
    } else if message.starts_with("Execution outcome is unknown:")
        || message.starts_with("RC command delivery failed:")
        || message == "Node process status timed out"
    {
        (
            "execution_outcome_unknown",
            "Query process_status with the device ID and process ID. Do not replay a command whose execution is uncertain.",
        )
    } else {
        (
            "tool_failed",
            "Inspect the returned error and use the RC reference to locate the server-side request. Do not assume the command is safe to replay.",
        )
    }
}

pub(super) fn tool_result(result: anyhow::Result<Value>, reference: &str) -> Value {
    let mut value = match result {
        Ok(value) => value,
        Err(error) => json!({
            "resultType": "complete", "isError": true,
            "content": [{"type": "text", "text": error.to_string()}],
        }),
    };
    let failed = value["isError"].as_bool().unwrap_or(false);
    let (code, guidance) = if !failed {
        ("ok", "")
    } else if value
        .pointer("/structuredContent/status")
        .and_then(Value::as_str)
        == Some("exited")
    {
        (
            "process_exit_nonzero",
            "The process exited unsuccessfully; this is not an RC connection failure.",
        )
    } else {
        let message = value
            .pointer("/structuredContent/error")
            .and_then(Value::as_str)
            .or_else(|| value.pointer("/content/0/text").and_then(Value::as_str))
            .unwrap_or_default();
        failure(message)
    };
    // MCP envelope metadata does not change the tool's exact outputSchema.
    value["_meta"]["party.ohrats.rc/diagnostic"] = json!({
        "requestId": reference, "code": code,
    });
    if failed {
        if let Some(content) = value["content"].as_array_mut() {
            content.push(json!({"type": "text", "text": format!("RC reference: {reference} ({code}). {guidance}")}));
        }
        tracing::warn!(error_code = code, "MCP tool failed");
    } else {
        tracing::info!(outcome = code, "MCP tool succeeded");
    }
    value
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn untrusted_labels_never_enter_diagnostics() {
        let body = br#"{"id":"secret","method":"secret","params":{"name":"secret","arguments":{"command":"secret"}}}"#;
        assert_eq!(labels(body), ("unknown", "unknown"));
        assert_eq!(failure("secret").0, "tool_failed");
    }

    #[test]
    fn outcome_metadata_preserves_output_schema_and_warns_against_replay() {
        let structured = json!({"status":"lost", "error":"Node process status timed out"});
        let value = tool_result(
            Ok(json!({"structuredContent":structured,"isError":true,"content":[]})),
            "reference",
        );
        assert_eq!(value["structuredContent"], structured);
        assert_eq!(
            value["_meta"]["party.ohrats.rc/diagnostic"]["code"],
            "execution_outcome_unknown"
        );
        assert!(
            value["content"][0]["text"]
                .as_str()
                .unwrap()
                .contains("Do not replay")
        );
    }

    #[test]
    fn nonzero_exit_is_distinct_from_transport_and_authorization_failure() {
        let value = tool_result(
            Ok(
                json!({"isError":true,"structuredContent":{"status":"exited","exitCode":7},"content":[]}),
            ),
            "reference",
        );
        assert_eq!(
            value["_meta"]["party.ohrats.rc/diagnostic"]["code"],
            "process_exit_nonzero"
        );
        assert_eq!(
            failure("device is outside this MCP grant").0,
            "grant_access_denied"
        );
        let value = tool_result(
            Err(anyhow::anyhow!(
                "Node is offline or still connecting; command was not sent"
            )),
            "reference",
        );
        assert_eq!(
            value["_meta"]["party.ohrats.rc/diagnostic"]["code"],
            "node_offline"
        );
    }
}
