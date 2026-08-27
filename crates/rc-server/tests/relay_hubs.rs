use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rc_protocol::NodeToServer;
use rc_server::{McpHub, SshHub, SshRelay};
use std::time::Duration;

#[tokio::test]
async fn ssh_hub_routes_only_the_bound_device_and_closes_on_exit() -> anyhow::Result<()> {
    let hub = SshHub::default();
    let mut receiver = hub.register("session", "device-a", "process");

    assert!(hub.handle(
        "device-b",
        &NodeToServer::SshStdout {
            session_id: "session".into(),
            data: URL_SAFE_NO_PAD.encode(b"wrong device"),
        },
    ));
    assert!(
        tokio::time::timeout(Duration::from_millis(25), receiver.recv())
            .await
            .is_err()
    );

    assert!(hub.handle(
        "device-a",
        &NodeToServer::SshStdout {
            session_id: "session".into(),
            data: URL_SAFE_NO_PAD.encode(b"stdout"),
        },
    ));
    match receiver.recv().await {
        Some(SshRelay::Stdout(data)) => assert_eq!(data, b"stdout"),
        other => anyhow::bail!("unexpected SSH stdout relay: {other:?}"),
    }

    assert!(hub.handle(
        "device-a",
        &NodeToServer::SshExit {
            session_id: "session".into(),
            exit_code: 7,
            signal: "TERM".into(),
        },
    ));
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
    hub.release_device("device");
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
async fn mcp_hub_is_grant_scoped_bounded_and_reconciles_exit() -> anyhow::Result<()> {
    let hub = McpHub::default();
    hub.register("process", "grant", "user", "device");

    assert!(hub.handle(
        "other-device",
        &NodeToServer::McpStdout {
            process_id: "process".into(),
            data: URL_SAFE_NO_PAD.encode(b"wrong"),
        },
    ));
    let empty = hub.result("process", "grant", "user", 0, 0).await?;
    assert!(empty.output.is_empty());

    let oversized = vec![b'x'; 256 * 1024 + 4096];
    assert!(hub.handle(
        "device",
        &NodeToServer::McpStdout {
            process_id: "process".into(),
            data: URL_SAFE_NO_PAD.encode(&oversized),
        },
    ));
    let output = hub.result("process", "grant", "user", 0, 0).await?;
    assert_eq!(output.output.len(), 256 * 1024);
    assert_eq!(output.next_offset, 256 * 1024);
    assert!(output.output_truncated);
    assert_eq!(output.status, "running");

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

    assert!(hub.handle(
        "device",
        &NodeToServer::McpExit {
            process_id: "process".into(),
            exit_code: 3,
            signal: String::new(),
        },
    ));
    let exited = hub
        .result("process", "grant", "user", 256 * 1024, 0)
        .await?;
    assert_eq!(exited.status, "exited");
    assert_eq!(exited.exit_code, Some(3));
    assert_eq!(exited.signal, None);
    Ok(())
}

#[tokio::test]
async fn mcp_hub_marks_running_processes_lost_on_disconnect() -> anyhow::Result<()> {
    let hub = McpHub::default();
    hub.register("process", "grant", "user", "device");
    hub.release_device("device");
    let result = hub.result("process", "grant", "user", 0, 0).await?;
    assert_eq!(result.status, "lost");
    assert_eq!(result.error.as_deref(), Some("RC Node disconnected"));
    Ok(())
}
