use rc_server::{AppState, RcEvent};
use std::time::Duration;
use tokio::time::timeout;

pub(super) async fn assert_presence(
    events: &mut tokio::sync::broadcast::Receiver<RcEvent>,
    kind: &str,
    device_id: &str,
) -> anyhow::Result<()> {
    let _ = assert_event(events, kind, device_id).await?;
    Ok(())
}

pub(super) async fn assert_event(
    events: &mut tokio::sync::broadcast::Receiver<RcEvent>,
    kind: &str,
    device_id: &str,
) -> anyhow::Result<RcEvent> {
    timeout(Duration::from_secs(3), async {
        loop {
            let event = events.recv().await?;
            if event.kind == kind && event.device_id.as_deref() == Some(device_id) {
                return Ok::<_, tokio::sync::broadcast::error::RecvError>(event);
            }
        }
    })
    .await
    .map_err(|_| anyhow::anyhow!("timed out waiting for {kind}"))?
    .map_err(Into::into)
}

pub(super) async fn assert_no_event(
    events: &mut tokio::sync::broadcast::Receiver<RcEvent>,
) -> anyhow::Result<()> {
    if let Ok(event) = timeout(Duration::from_millis(150), events.recv()).await {
        anyhow::bail!("unexpected duplicate lifecycle event: {:?}", event?);
    }
    Ok(())
}

pub(super) async fn wait_online(
    state: &AppState,
    device_id: &str,
    expected: bool,
) -> anyhow::Result<()> {
    timeout(Duration::from_secs(3), async {
        loop {
            if state.nodes.online(device_id).await == expected {
                return;
            }
            tokio::time::sleep(Duration::from_millis(20)).await;
        }
    })
    .await
    .map_err(|_| anyhow::anyhow!("Node online state did not become {expected}"))?;
    Ok(())
}

pub(super) async fn wait_closed(
    closed: &mut tokio::sync::watch::Receiver<bool>,
) -> anyhow::Result<()> {
    timeout(Duration::from_secs(3), async {
        while !*closed.borrow() {
            closed.changed().await?;
        }
        Ok::<(), tokio::sync::watch::error::RecvError>(())
    })
    .await??;
    Ok(())
}
