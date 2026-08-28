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
async fn mcp_hub_preserves_stream_order_and_incremental_cursors() -> anyhow::Result<()> {
    let hub = McpHub::default();
    hub.register("process", "grant", "user", "device")?;

    hub.handle(
        "other-device",
        &NodeToServer::McpStdout {
            process_id: "process".into(),
            data: URL_SAFE_NO_PAD.encode(b"wrong"),
        },
    );
    let empty = hub.result("process", "grant", "user", 0, 0).await?;
    assert!(empty.chunks.is_empty());

    hub.handle(
        "device",
        &NodeToServer::McpStdout {
            process_id: "process".into(),
            data: URL_SAFE_NO_PAD.encode(b"out"),
        },
    );
    hub.handle(
        "device",
        &NodeToServer::McpStderr {
            process_id: "process".into(),
            data: URL_SAFE_NO_PAD.encode(b"err"),
        },
    );
    let output = hub.result("process", "grant", "user", 0, 0).await?;
    assert_eq!(output.chunks.len(), 2);
    assert_eq!(output.chunks[0].stream, "stdout");
    assert_eq!(output.chunks[0].text, "out");
    assert_eq!(output.chunks[1].stream, "stderr");
    assert_eq!(output.chunks[1].text, "err");
    assert_eq!(output.next_cursor, 6);
    assert_eq!(output.status, "running");
    assert!(!output.output_pending);

    let incremental = hub.result("process", "grant", "user", 3, 0).await?;
    assert_eq!(incremental.chunks.len(), 1);
    assert_eq!(incremental.chunks[0].stream, "stderr");
    assert_eq!(incremental.chunks[0].text, "err");
    assert_eq!(incremental.next_cursor, 6);

    assert!(
        hub.result("process", "other-grant", "user", 0, 0)
            .await
            .is_err()
    );
    assert!(
        hub.result("process", "grant", "other-user", 0, 0)
            .await
            .is_err()
    );

    assert_eq!(
        hub.handle(
            "device",
            &NodeToServer::McpExit {
                process_id: "process".into(),
                exit_code: 3,
                signal: String::new(),
            },
        ),
        Some(("process".into(), 3, String::new()))
    );
    let exited = hub.result("process", "grant", "user", 6, 0).await?;
    assert_eq!(exited.status, "exited");
    assert_eq!(exited.exit_code, Some(3));
    assert_eq!(exited.signal, None);
    Ok(())
}

#[tokio::test]
async fn mcp_hub_rolls_output_forward_instead_of_freezing() -> anyhow::Result<()> {
    let hub = McpHub::default();
    hub.register("process", "grant", "user", "device")?;
    let chunk = vec![b'x'; 16 * 1024];
    for _ in 0..20 {
        hub.handle(
            "device",
            &NodeToServer::McpStdout {
                process_id: "process".into(),
                data: URL_SAFE_NO_PAD.encode(&chunk),
            },
        );
    }
    let total = 20 * 16 * 1024;
    let earliest = total - 256 * 1024;
    let first = hub.result("process", "grant", "user", 0, 0).await?;
    assert_eq!(first.truncated_before_cursor, earliest);
    assert_eq!(first.next_cursor, earliest + 64 * 1024);
    assert!(first.output_pending);

    let newest = hub
        .result("process", "grant", "user", total - 16 * 1024, 0)
        .await?;
    assert_eq!(newest.chunks.len(), 1);
    assert_eq!(newest.chunks[0].text.len(), 16 * 1024);
    assert_eq!(newest.next_cursor, total);
    assert!(!newest.output_pending);
    Ok(())
}

#[tokio::test]
async fn mcp_hub_marks_running_processes_lost_on_disconnect() -> anyhow::Result<()> {
    let hub = McpHub::default();
    hub.register("process", "grant", "user", "device")?;
    assert_eq!(hub.release_device("device"), vec!["process"]);
    let result = hub.result("process", "grant", "user", 0, 0).await?;
    assert_eq!(result.status, "lost");
    assert_eq!(result.error.as_deref(), Some("RC Node disconnected"));
    Ok(())
}

#[test]
fn mcp_cancel_target_is_bound_to_the_originating_grant_and_user() -> anyhow::Result<()> {
    let hub = McpHub::default();
    hub.register("process", "grant", "user", "device")?;

    assert_eq!(hub.running_device("process", "grant", "user")?, "device");
    assert!(
        hub.running_device("process", "other-grant", "user")
            .is_err()
    );
    assert!(
        hub.running_device("process", "grant", "other-user")
            .is_err()
    );
    Ok(())
}

#[test]
fn mcp_hub_never_evicts_a_live_process_to_make_room() -> anyhow::Result<()> {
    let hub = McpHub::default();
    for index in 0..128 {
        hub.register(&format!("process-{index}"), "grant", "user", "device")?;
    }
    assert!(hub.register("overflow", "grant", "user", "device").is_err());
    assert_eq!(hub.running_device("process-0", "grant", "user")?, "device");
    Ok(())
}
