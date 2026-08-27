use super::{fixture::Harness, support};
use rc_node::ServerTransport;
use rc_protocol::{AuthoritySnapshot, NodeHello, NodeToServer, ServerToNode};
use std::time::Duration;
use tokio::time::timeout;

pub(super) async fn connect(
    harness: &mut Harness,
    presence: &mut tokio::sync::broadcast::Receiver<rc_server::RcEvent>,
) -> anyhow::Result<ServerTransport> {
    let mut inbound = harness.state.nodes.subscribe();
    let mut transport = harness.connect().await?;
    support::wait_online(&harness.state, &harness.device_id, true).await?;
    support::assert_presence(presence, "device.online", &harness.device_id).await?;

    let hello = NodeHello {
        version: "0.16.0-test".into(),
        hostname: "node-host".into(),
        platform: "darwin".into(),
        arch: "arm64".into(),
        capabilities: vec!["process".into(), "webrtc".into()],
        transport_public_key: harness.node.transport_public_key()?,
        lock_hash: String::new(),
        lock_generation: 0,
    };
    let hello_message = NodeToServer::Hello {
        hello: hello.clone(),
    };
    transport.send(&hello_message).await?;
    let received = timeout(Duration::from_secs(3), inbound.recv())
        .await
        .map_err(|_| anyhow::anyhow!("timed out waiting for Node-to-server hello"))??;
    assert_eq!(received.device_id, harness.device_id);
    assert_eq!(received.message, hello_message);

    let bootstrap = timeout(Duration::from_secs(3), transport.recv())
        .await
        .map_err(|_| anyhow::anyhow!("timed out waiting for RC Lock bootstrap"))?;
    let Some(ServerToNode::LockBootstrap { snapshot }) = bootstrap else {
        anyhow::bail!("expected RC Lock bootstrap, got {bootstrap:?}");
    };
    let snapshot: AuthoritySnapshot = serde_json::from_str(&snapshot)?;
    assert_eq!(snapshot.v, 1);
    assert_eq!(snapshot.workspace_id, harness.workspace_id);

    let message = ServerToNode::ControlChallenge {
        request_id: "first".into(),
    };
    harness
        .state
        .nodes
        .send(&harness.device_id, &message)
        .await?;
    assert_eq!(
        timeout(Duration::from_secs(3), transport.recv())
            .await
            .map_err(|_| anyhow::anyhow!("timed out waiting for first server-to-Node message"))?,
        Some(message)
    );
    Ok(transport)
}
