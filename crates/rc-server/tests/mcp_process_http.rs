#[path = "mcp_process_http/support.rs"]
mod support;

use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_node::ServerTransport;
use rc_protocol::{NodeToServer, ServerToNode};
use std::time::Duration;
use support::{Harness, recv, rpc_call};
use tokio::time::timeout;

#[tokio::test]
async fn mcp_process_harness_runs_reads_writes_and_cancels() -> anyhow::Result<()> {
    let harness = Harness::start().await?;
    let client = reqwest::Client::new();
    let mut node = timeout(
        Duration::from_secs(10),
        ServerTransport::connect(&harness.base, &harness.node),
    )
    .await??;

    let run = rpc_call(
        &client,
        &harness,
        "process_run",
        serde_json::json!({
            "deviceId": harness.device_id,
            "command": "python3 -u -c 'n=input(\"number: \"); print(int(n)*2)'",
            "waitSeconds": 0,
        }),
        1,
    )
    .await?;
    let process_id = run["result"]["structuredContent"]["processId"]
        .as_str()
        .ok_or_else(|| anyhow::anyhow!("missing process ID"))?
        .to_owned();
    assert_eq!(run["result"]["structuredContent"]["status"], "running");
    match recv(&mut node).await? {
        ServerToNode::McpStart {
            process_id: id,
            command,
            cwd,
            ..
        } => {
            assert_eq!(id, process_id);
            assert!(command.contains("input"));
            assert!(cwd.is_empty());
        }
        message => anyhow::bail!("expected MCP start, got {message:?}"),
    }

    node.send(&NodeToServer::McpStdout {
        process_id: process_id.clone(),
        data: URL_SAFE_NO_PAD.encode(b"number: "),
    })
    .await?;
    let prompt = rpc_call(
        &client,
        &harness,
        "process_status",
        serde_json::json!({"processId": process_id, "cursor": 0, "waitSeconds": 2}),
        2,
    )
    .await?;
    let prompt_state = &prompt["result"]["structuredContent"];
    assert_eq!(prompt_state["chunks"][0]["stream"], "stdout");
    assert_eq!(prompt_state["chunks"][0]["text"], "number: ");
    let cursor = prompt_state["nextCursor"].as_u64().unwrap_or_default();

    let input = rpc_call(
        &client,
        &harness,
        "process_input",
        serde_json::json!({"processId": process_id, "data": "42\n", "eof": true}),
        3,
    )
    .await?;
    assert_eq!(input["result"]["structuredContent"]["acceptedBytes"], 3);
    assert_eq!(input["result"]["structuredContent"]["stdinClosed"], true);
    match recv(&mut node).await? {
        ServerToNode::McpStdin {
            process_id: id,
            data,
        } => {
            assert_eq!(id, process_id);
            assert_eq!(URL_SAFE_NO_PAD.decode(data)?, b"42\n");
        }
        message => anyhow::bail!("expected MCP stdin, got {message:?}"),
    }
    assert_eq!(
        recv(&mut node).await?,
        ServerToNode::McpStdinClose {
            process_id: process_id.clone(),
        }
    );

    node.send(&NodeToServer::McpStdout {
        process_id: process_id.clone(),
        data: URL_SAFE_NO_PAD.encode(b"84\n"),
    })
    .await?;
    node.send(&NodeToServer::McpExit {
        process_id: process_id.clone(),
        exit_code: 0,
        signal: String::new(),
    })
    .await?;
    let finished = rpc_call(
        &client,
        &harness,
        "process_status",
        serde_json::json!({"processId": process_id, "cursor": cursor, "waitSeconds": 2}),
        4,
    )
    .await?;
    let final_state = &finished["result"]["structuredContent"];
    assert_eq!(final_state["status"], "exited");
    assert_eq!(final_state["chunks"][0]["text"], "84\n");
    assert_eq!(final_state["exitCode"], 0);

    exercise_cancel(&client, &harness, &mut node).await?;
    exercise_image_view(&client, &harness, &mut node).await?;
    assert_node_transport_limit(&client, &harness, &mut node).await?;
    assert_http_request_limit(&client, &harness).await?;
    node.close().await;
    Ok(())
}

async fn assert_node_transport_limit(
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

async fn assert_http_request_limit(
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

async fn exercise_cancel(
    client: &reqwest::Client,
    harness: &Harness,
    node: &mut ServerTransport,
) -> anyhow::Result<()> {
    let run = rpc_call(
        client,
        harness,
        "process_run",
        serde_json::json!({
            "deviceId": harness.device_id,
            "command": "sleep 600",
            "waitSeconds": 0,
        }),
        5,
    )
    .await?;
    let process_id = run["result"]["structuredContent"]["processId"]
        .as_str()
        .ok_or_else(|| anyhow::anyhow!("missing cancellation process ID"))?
        .to_owned();
    assert!(matches!(recv(node).await?, ServerToNode::McpStart { .. }));
    let cancelled = rpc_call(
        client,
        harness,
        "process_cancel",
        serde_json::json!({"processId": process_id}),
        6,
    )
    .await?;
    assert_eq!(cancelled["result"]["structuredContent"]["signal"], "TERM");
    assert_eq!(
        recv(node).await?,
        ServerToNode::McpSignal {
            process_id: process_id.clone(),
            signal: "TERM".into(),
        }
    );
    node.send(&NodeToServer::McpExit {
        process_id: process_id.clone(),
        exit_code: -1,
        signal: "TERM".into(),
    })
    .await?;
    let status = rpc_call(
        client,
        harness,
        "process_status",
        serde_json::json!({"processId": process_id, "waitSeconds": 2}),
        7,
    )
    .await?;
    assert_eq!(status["result"]["structuredContent"]["status"], "exited");
    assert_eq!(status["result"]["structuredContent"]["signal"], "TERM");
    Ok(())
}

async fn exercise_image_view(
    client: &reqwest::Client,
    harness: &Harness,
    node: &mut ServerTransport,
) -> anyhow::Result<()> {
    let image_bytes = b"fake-png-bytes";
    let call = rpc_call(
        client,
        harness,
        "image_view",
        serde_json::json!({"deviceId": harness.device_id, "path": "/tmp/shot.png"}),
        10,
    );
    let respond = async {
        let request_id = match recv(node).await? {
            ServerToNode::McpImageView {
                request_id,
                path,
                user_id,
                ..
            } => {
                assert_eq!(path, "/tmp/shot.png");
                assert!(!user_id.is_empty());
                request_id
            }
            message => anyhow::bail!("expected MCP image view, got {message:?}"),
        };
        node.send(&NodeToServer::McpImageChunk {
            request_id: request_id.clone(),
            data: URL_SAFE_NO_PAD.encode(image_bytes),
        })
        .await?;
        node.send(&NodeToServer::McpImageResult {
            request_id,
            mime_type: "image/png".into(),
            size_bytes: image_bytes.len() as u64,
            error: String::new(),
        })
        .await?;
        Ok::<(), anyhow::Error>(())
    };
    let (result, ()) = tokio::try_join!(call, respond)?;
    let output = &result["result"];
    assert_eq!(output["structuredContent"]["mimeType"], "image/png");
    assert_eq!(output["structuredContent"]["sizeBytes"], image_bytes.len());
    let image = output["content"]
        .as_array()
        .and_then(|items| items.iter().find(|item| item["type"] == "image"))
        .ok_or_else(|| anyhow::anyhow!("missing image content"))?;
    assert_eq!(image["mimeType"], "image/png");
    assert_eq!(
        base64::engine::general_purpose::STANDARD
            .decode(image["data"].as_str().unwrap_or_default())?,
        image_bytes
    );
    Ok(())
}
