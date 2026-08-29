use super::{fixture::Harness, support};
use rc_node::{NodeRuntime, ServerTransport};
use rc_protocol::{NodeToServer, ServerToNode};
use std::{path::PathBuf, time::Duration};
use tokio::time::timeout;
use uuid::Uuid;

pub(super) async fn exercise(
    harness: &mut Harness,
    first: ServerTransport,
    presence: &mut tokio::sync::broadcast::Receiver<rc_server::RcEvent>,
) -> anyhow::Result<()> {
    let mut first_closed = first.closed();
    let mut second = harness.connect().await?;
    support::wait_closed(&mut first_closed).await?;
    support::wait_online(&harness.state, &harness.device_id, true).await?;
    support::assert_presence(presence, "device.offline", &harness.device_id).await?;
    support::assert_presence(presence, "device.online", &harness.device_id).await?;
    assert!(!harness.state.control.has_session("control-session"));

    let replacement_message = ServerToNode::ControlChallenge {
        request_id: "replacement".into(),
    };
    harness
        .state
        .nodes
        .send(&harness.device_id, &replacement_message)
        .await?;
    assert_eq!(
        timeout(Duration::from_secs(3), second.recv())
            .await
            .map_err(|_| anyhow::anyhow!(
                "timed out waiting for replacement server-to-Node message"
            ))?,
        Some(replacement_message)
    );

    second.close().await;
    support::wait_online(&harness.state, &harness.device_id, false).await?;
    support::assert_presence(presence, "device.offline", &harness.device_id).await?;

    let mut third = harness.connect().await?;
    support::wait_online(&harness.state, &harness.device_id, true).await?;
    support::assert_presence(presence, "device.online", &harness.device_id).await?;
    let reconnect_message = ServerToNode::ControlChallenge {
        request_id: "reconnect".into(),
    };
    harness
        .state
        .nodes
        .send(&harness.device_id, &reconnect_message)
        .await?;
    assert_eq!(
        timeout(Duration::from_secs(3), third.recv())
            .await
            .map_err(|_| anyhow::anyhow!(
                "timed out waiting for reconnect server-to-Node message"
            ))?,
        Some(reconnect_message)
    );
    third.close().await;
    support::wait_online(&harness.state, &harness.device_id, false).await?;
    support::assert_presence(presence, "device.offline", &harness.device_id).await?;

    let mut runtime_events = harness.state.nodes.subscribe();
    let runtime_base = harness.base.clone();
    let runtime_node = harness.node.clone();
    let runtime_task = tokio::spawn(async move {
        let (process_policy, transport_policy) = crate::policies::pair();
        let mut runtime = NodeRuntime::new(
            PathBuf::from("/unused-process-runner"),
            std::env::temp_dir().join(format!("rc-runtime-state-{}", Uuid::new_v4())),
            process_policy,
            transport_policy,
        );
        runtime
            .connect_once(&runtime_base, &runtime_node, "runtime-test")
            .await
    });
    let runtime_hello = timeout(Duration::from_secs(3), runtime_events.recv())
        .await
        .map_err(|_| anyhow::anyhow!("timed out waiting for runtime hello"))??;
    match runtime_hello.message {
        NodeToServer::Hello { hello } => {
            assert_eq!(hello.version, "runtime-test");
            assert_eq!(
                hello.transport_public_key,
                harness.node.transport_public_key()?
            );
        }
        other => anyhow::bail!("expected runtime hello, got {other:?}"),
    }
    let runtime_sync = timeout(Duration::from_secs(3), runtime_events.recv())
        .await
        .map_err(|_| anyhow::anyhow!("timed out waiting for runtime process sync"))??;
    assert_eq!(
        runtime_sync.message,
        NodeToServer::ProcessSync { ids: Vec::new() }
    );

    let replacement = harness.connect().await?;
    timeout(Duration::from_secs(3), runtime_task)
        .await
        .map_err(|_| anyhow::anyhow!("runtime did not exit after connection replacement"))???;
    replacement.close().await;
    Ok(())
}
