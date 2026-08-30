use super::support::{Harness, recv, respond_status, rpc_call};
use rc_node::ServerTransport;
use rc_protocol::{ExecutionMode, NodeToServer, ServerToNode};
use std::time::Duration;
use tokio::time::timeout;

pub async fn exercise_exact_argv(
    client: &reqwest::Client,
    harness: &Harness,
    node: &mut ServerTransport,
) -> anyhow::Result<()> {
    let argv = serde_json::json!(["fixture", "", "hello world", "'", "line\nbreak", "🐀"]);
    let run_call = rpc_call(
        client,
        harness,
        "process_run",
        serde_json::json!({
            "deviceId": harness.device_id,
            "argv": argv,
            "envBase": "clean",
            "env": {"Path": "C:\\Tools", "EMPTY": "", "REMOVE": null},
            "maxRuntimeSeconds": 30,
            "waitSeconds": 0,
        }),
        10,
    );
    let start_exchange = async {
        let start = recv(node).await?;
        let ServerToNode::McpStart { process_id, .. } = &start else {
            anyhow::bail!("expected exact argv MCP start, got {start:?}");
        };
        respond_status(node, process_id, "running", Vec::new(), None, "").await?;
        Ok::<_, anyhow::Error>(start)
    };
    let (run, start) = tokio::try_join!(run_call, start_exchange)?;
    let process_id = run["result"]["structuredContent"]["processId"]
        .as_str()
        .ok_or_else(|| anyhow::anyhow!("missing argv process ID"))?
        .to_owned();
    match start {
        ServerToNode::McpStart {
            process_id: id,
            mode: ExecutionMode::Argv { program, args },
            environment,
            max_runtime_seconds,
            ..
        } => {
            assert_eq!(id, process_id);
            assert_eq!(program, "fixture");
            assert_eq!(args, vec!["", "hello world", "'", "line\nbreak", "🐀"]);
            assert_eq!(environment.base, rc_protocol::EnvironmentBase::Clean);
            assert_eq!(environment.changes.len(), 3);
            assert_eq!(max_runtime_seconds, Some(30));
        }
        message => anyhow::bail!("expected exact argv MCP start, got {message:?}"),
    }
    node.send(&NodeToServer::McpExit {
        process_id,
        exit_code: 0,
        signal: String::new(),
    })
    .await?;
    Ok(())
}

pub async fn assert_http_request_limit(
    client: &reqwest::Client,
    harness: &Harness,
) -> anyhow::Result<()> {
    let command = "x".repeat(2 * 1024 * 1024);
    let response = client
        .post(format!("{}/mcp", harness.base))
        .bearer_auth(&harness.access_token)
        .header("mcp-protocol-version", rc_server::MCP_PROTOCOL_VERSION)
        .header("mcp-method", "tools/call")
        .header("mcp-name", "process_run")
        .json(&serde_json::json!({
            "jsonrpc": "2.0",
            "id": 9,
            "method": "tools/call",
            "params": {
                "name": "process_run",
                "arguments": {"deviceId": harness.device_id, "command": command},
            },
        }))
        .send()
        .await?;
    assert_eq!(response.status(), reqwest::StatusCode::PAYLOAD_TOO_LARGE);
    Ok(())
}

pub async fn assert_node_transport_limit(
    client: &reqwest::Client,
    harness: &Harness,
    node: &mut ServerTransport,
) -> anyhow::Result<()> {
    let command = "x".repeat(rc_protocol::NODE_CONTROL_MESSAGE_LIMIT + 1024);
    let result = rpc_call(
        client,
        harness,
        "process_run",
        serde_json::json!({
            "deviceId": harness.device_id,
            "command": command,
            "waitSeconds": 0,
        }),
        8,
    )
    .await?;
    let state = &result["result"]["structuredContent"];
    assert_eq!(state["status"], "lost");
    assert!(
        state["error"]
            .as_str()
            .is_some_and(|error| error.contains("transport frame"))
    );
    assert!(
        timeout(Duration::from_millis(100), node.recv())
            .await
            .is_err()
    );
    Ok(())
}
