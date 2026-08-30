#[path = "mcp_process_http/execution_modes.rs"]
mod execution_modes;
#[path = "mcp_process_http/support.rs"]
mod support;

use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_node::ServerTransport;
use rc_protocol::{ExecutionMode, NodeToServer, ServerToNode};
use std::time::Duration;
use support::{Harness, accept_operation, recv, respond_status, rpc_call};
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

    let run_call = rpc_call(
        &client,
        &harness,
        "process_run",
        serde_json::json!({
            "deviceId": harness.device_id,
            "command": "python3 -u -c 'n=input(\"number: \"); print(int(n)*2)'",
            "waitSeconds": 0,
        }),
        1,
    );
    let start_exchange = async {
        let start = recv(&mut node).await?;
        let ServerToNode::McpStart { process_id, .. } = &start else {
            anyhow::bail!("expected MCP start, got {start:?}");
        };
        respond_status(&mut node, process_id, "running", Vec::new(), None, "").await?;
        Ok::<_, anyhow::Error>(start)
    };
    let (run, start) = tokio::try_join!(run_call, start_exchange)?;
    let process_id = run["result"]["structuredContent"]["processId"]
        .as_str()
        .ok_or_else(|| anyhow::anyhow!("missing process ID"))?
        .to_owned();
    assert_eq!(run["result"]["structuredContent"]["status"], "running");
    match start {
        ServerToNode::McpStart {
            process_id: id,
            mode,
            cwd,
            ..
        } => {
            assert_eq!(id, process_id);
            assert!(matches!(mode, ExecutionMode::RcShell { script } if script.contains("input")));
            assert!(cwd.is_empty());
        }
        message => anyhow::bail!("expected MCP start, got {message:?}"),
    }
    let prompt_call = rpc_call(
        &client,
        &harness,
        "process_status",
        serde_json::json!({"deviceId": harness.device_id, "processId": process_id, "cursor": 0, "waitSeconds": 2}),
        2,
    );
    let prompt_status = respond_status(
        &mut node,
        &process_id,
        "running",
        vec![("stdout", b"number: ".to_vec())],
        None,
        "",
    );
    let (prompt, ()) = tokio::try_join!(prompt_call, prompt_status)?;
    let prompt_state = &prompt["result"]["structuredContent"];
    assert_eq!(prompt_state["chunks"][0]["stream"], "stdout");
    assert_eq!(prompt_state["chunks"][0]["cursor"], 0);
    assert_eq!(prompt_state["chunks"][0]["encoding"], "text");
    assert_eq!(prompt_state["chunks"][0]["data"], "number: ");
    let cursor = prompt_state["nextCursor"].as_u64().unwrap_or_default();

    let input_call = rpc_call(
        &client,
        &harness,
        "process_input",
        serde_json::json!({"deviceId": harness.device_id, "processId": process_id, "data": "42\n", "eof": true}),
        3,
    );
    let input_exchange = accept_operation(&mut node);
    let (input, input_message) = tokio::try_join!(input_call, input_exchange)?;
    assert_eq!(input["result"]["structuredContent"]["acceptedBytes"], 3);
    assert_eq!(input["result"]["structuredContent"]["stdinClosed"], true);
    match input_message {
        ServerToNode::McpExecutionInput {
            process_id: id,
            data,
            eof,
            user_id,
            mcp_grant,
            control_grant,
            ..
        } => {
            assert_eq!(id, process_id);
            assert_eq!(URL_SAFE_NO_PAD.decode(data)?, b"42\n");
            assert!(eof);
            assert!(!user_id.is_empty());
            assert!(!mcp_grant.is_empty());
            assert!(!control_grant.is_empty());
        }
        message => anyhow::bail!("expected MCP stdin, got {message:?}"),
    }

    node.send(&NodeToServer::McpExit {
        process_id: process_id.clone(),
        exit_code: 0,
        signal: String::new(),
    })
    .await?;
    let finished_call = rpc_call(
        &client,
        &harness,
        "process_status",
        serde_json::json!({"deviceId": harness.device_id, "processId": process_id, "cursor": cursor, "waitSeconds": 2}),
        4,
    );
    let finished_status = respond_status(
        &mut node,
        &process_id,
        "exited",
        vec![("stdout", b"84\n".to_vec())],
        Some(0),
        "",
    );
    let (finished, ()) = tokio::try_join!(finished_call, finished_status)?;
    let final_state = &finished["result"]["structuredContent"];
    assert_eq!(final_state["status"], "exited");
    assert_eq!(final_state["chunks"][0]["data"], "84\n");
    assert_eq!(final_state["exitCode"], 0);

    let binary_cursor = final_state["nextCursor"].as_u64().unwrap_or_default();
    let binary_call = rpc_call(
        &client,
        &harness,
        "process_status",
        serde_json::json!({"deviceId": harness.device_id, "processId": process_id, "cursor": binary_cursor}),
        40,
    );
    let binary_status = respond_status(
        &mut node,
        &process_id,
        "exited",
        vec![("stdout", vec![0xff, 0x00, 0x80])],
        Some(0),
        "",
    );
    let (binary, ()) = tokio::try_join!(binary_call, binary_status)?;
    let chunk = &binary["result"]["structuredContent"]["chunks"][0];
    assert_eq!(chunk["encoding"], "base64");
    assert_eq!(chunk["data"], "/wCA");

    execution_modes::exercise_exact_argv(&client, &harness, &mut node).await?;
    exercise_cancel(&client, &harness, &mut node).await?;
    exercise_image_view(&client, &harness, &mut node).await?;
    execution_modes::assert_node_transport_limit(&client, &harness, &mut node).await?;
    execution_modes::assert_http_request_limit(&client, &harness).await?;
    node.close().await;
    Ok(())
}

async fn exercise_cancel(
    client: &reqwest::Client,
    harness: &Harness,
    node: &mut ServerTransport,
) -> anyhow::Result<()> {
    let run_call = rpc_call(
        client,
        harness,
        "process_run",
        serde_json::json!({
            "deviceId": harness.device_id,
            "command": "sleep 600",
            "waitSeconds": 0,
        }),
        5,
    );
    let start_exchange = async {
        let start = recv(node).await?;
        let ServerToNode::McpStart { process_id, .. } = &start else {
            anyhow::bail!("expected cancellation MCP start");
        };
        respond_status(node, process_id, "running", Vec::new(), None, "").await?;
        Ok::<_, anyhow::Error>(start)
    };
    let (run, start) = tokio::try_join!(run_call, start_exchange)?;
    let process_id = run["result"]["structuredContent"]["processId"]
        .as_str()
        .ok_or_else(|| anyhow::anyhow!("missing cancellation process ID"))?
        .to_owned();
    assert!(matches!(start, ServerToNode::McpStart { .. }));
    let cancelled_call = rpc_call(
        client,
        harness,
        "process_cancel",
        serde_json::json!({"deviceId": harness.device_id, "processId": process_id}),
        6,
    );
    let cancel_exchange = accept_operation(node);
    let (cancelled, cancel_message) = tokio::try_join!(cancelled_call, cancel_exchange)?;
    assert_eq!(cancelled["result"]["structuredContent"]["signal"], "TERM");
    assert!(matches!(
        cancel_message,
        ServerToNode::McpExecutionSignal { process_id: id, signal, .. }
            if id == process_id && signal == "TERM"
    ));
    node.send(&NodeToServer::McpExit {
        process_id: process_id.clone(),
        exit_code: -1,
        signal: "TERM".into(),
    })
    .await?;
    let status_call = rpc_call(
        client,
        harness,
        "process_status",
        serde_json::json!({"deviceId": harness.device_id, "processId": process_id, "waitSeconds": 2}),
        7,
    );
    let status_response = respond_status(node, &process_id, "exited", Vec::new(), Some(-1), "TERM");
    let (status, ()) = tokio::try_join!(status_call, status_response)?;
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
