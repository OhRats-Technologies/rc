use rc_server::NodeHub;
use std::{sync::Arc, time::Duration};
use webrtc::api::APIBuilder;

#[tokio::test]
async fn pending_connection_waits_and_stale_close_keeps_replacement() -> anyhow::Result<()> {
    let hub = NodeHub::default();
    let api = APIBuilder::new().build();
    let peer = Arc::new(api.new_peer_connection(Default::default()).await?);
    hub.insert_pending("device", "current".into(), peer.clone())
        .await;
    assert!(
        tokio::time::timeout(Duration::from_millis(100), hub.wait_ready("device"))
            .await
            .is_err()
    );
    assert!(!hub.remove_if("device", "stale").await);
    let channel = peer.create_data_channel("test", None).await?;
    assert!(hub.set_channel("device", "current", channel).await);
    hub.wait_ready("device").await?;
    assert!(hub.online("device").await);
    assert!(hub.remove_if("device", "current").await);
    Ok(())
}
