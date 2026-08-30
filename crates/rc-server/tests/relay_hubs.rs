use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_protocol::NodeToServer;
use rc_server::{McpHub, SshHub, SshRelay};
use std::time::Duration;

#[tokio::test]
async fn ssh_hub_routes_only_the_bound_device_and_closes_on_exit() -> anyhow::Result<()> {
    let hub = SshHub::default();
    let mut receiver = hub.register("session", "device-a", "process");

    assert!(
        hub.handle(
            "device-b",
            &NodeToServer::SshStdout {
                session_id: "session".into(),
                data: URL_SAFE_NO_PAD.encode(b"wrong device"),
            },
        )
        .is_none()
    );
    assert!(
        tokio::time::timeout(Duration::from_millis(25), receiver.recv())
            .await
            .is_err()
    );

    assert!(
        hub.handle(
            "device-a",
            &NodeToServer::SshStdout {
                session_id: "session".into(),
                data: URL_SAFE_NO_PAD.encode(b"stdout"),
            },
        )
        .is_none()
    );
    match receiver.recv().await {
        Some(SshRelay::Stdout(data)) => assert_eq!(data, b"stdout"),
        other => anyhow::bail!("unexpected SSH stdout relay: {other:?}"),
    }

    assert_eq!(
        hub.handle(
            "device-a",
            &NodeToServer::SshExit {
                session_id: "session".into(),
                exit_code: 7,
                signal: "TERM".into(),
            },
        ),
        Some(("process".into(), 7, "TERM".into()))
    );
    match receiver.recv().await {
        Some(SshRelay::Exit { code, signal }) => {
            assert_eq!(code, 7);
            assert_eq!(signal, "TERM");
        }
        other => anyhow::bail!("unexpected SSH exit relay: {other:?}"),
    }
    assert_eq!(hub.remove("session"), None);
    Ok(())
}

#[tokio::test]
async fn ssh_hub_marks_live_sessions_disconnected() -> anyhow::Result<()> {
    let hub = SshHub::default();
    let mut receiver = hub.register("session", "device", "process");
    assert_eq!(hub.release_device("device"), vec!["process"]);
    match receiver.recv().await {
        Some(SshRelay::Exit { code, signal }) => {
            assert_eq!(code, 255);
            assert_eq!(signal, "DISCONNECTED");
        }
        other => anyhow::bail!("unexpected SSH disconnect relay: {other:?}"),
    }
    Ok(())
}

#[tokio::test]
async fn mcp_hub_correlates_status_without_owning_output() -> anyhow::Result<()> {
    let hub = McpHub::default();
    let (request_id, receiver) = hub.status_request("device")?;
    let response = NodeToServer::McpExecutionStatusResult {
        request_id,
        process_id: "process".into(),
        status: "running".into(),
        chunks: vec![rc_protocol::McpExecutionChunk {
            stream: "stdout".into(),
            cursor: 0,
            data: URL_SAFE_NO_PAD.encode(b"transit only"),
        }],
        next_cursor: 12,
        truncated_before_cursor: 0,
        output_pending: false,
        exit_code: None,
        signal: String::new(),
        error: String::new(),
    };
    hub.handle("device", &response);
    assert_eq!(receiver.await?, response);
    Ok(())
}

#[test]
fn mcp_hub_owns_no_process_state_across_disconnect() {
    let hub = McpHub::default();
    assert!(hub.release_device("device").is_empty());
}

#[tokio::test]
async fn mcp_status_recorrelates_after_hosted_hub_restart() -> anyhow::Result<()> {
    let original = McpHub::default();
    let (_abandoned_id, abandoned) = original.status_request("device")?;
    drop(original);
    assert!(abandoned.await.is_err());

    let restarted = McpHub::default();
    let (request_id, receiver) = restarted.status_request("device")?;
    let response = NodeToServer::McpExecutionStatusResult {
        request_id,
        process_id: "node-owned-process".into(),
        status: "running".into(),
        chunks: Vec::new(),
        next_cursor: 41,
        truncated_before_cursor: 9,
        output_pending: false,
        exit_code: None,
        signal: String::new(),
        error: String::new(),
    };
    restarted.handle("device", &response);
    assert_eq!(receiver.await?, response);
    Ok(())
}

#[test]
fn mcp_exit_is_forwarded_without_a_server_process_registry() {
    let hub = McpHub::default();
    assert_eq!(
        hub.handle(
            "device",
            &NodeToServer::McpExit {
                process_id: "process".into(),
                exit_code: 0,
                signal: String::new(),
            },
        ),
        Some(("process".into(), 0, String::new()))
    );
}

#[test]
fn mcp_correlation_capacity_rejects_new_work_without_evicting_pending_requests() {
    let hub = McpHub::default();
    let mut first = None;
    for index in 0..4_096 {
        let (request_id, receiver) = hub.status_request("device").unwrap();
        if index == 0 {
            first = Some(request_id);
        }
        drop(receiver);
    }
    assert!(hub.status_request("device").is_err());
    hub.cancel_status_request(first.as_deref().unwrap());
    assert!(hub.status_request("device").is_ok());
}
