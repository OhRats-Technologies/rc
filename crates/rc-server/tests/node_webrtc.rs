#[path = "node_webrtc/bootstrap.rs"]
mod bootstrap;
#[path = "node_webrtc/control.rs"]
mod control;
#[path = "node_webrtc/fixture.rs"]
mod fixture;
#[path = "node_webrtc/lifecycle.rs"]
mod lifecycle;
#[path = "node_webrtc/reconnect.rs"]
mod reconnect;
#[path = "node_webrtc/support.rs"]
mod support;

use fixture::Harness;

#[tokio::test]
async fn node_signed_http_bootstrap_and_webrtc_reconnect() -> anyhow::Result<()> {
    let mut harness = Harness::start().await?;
    let mut presence = harness.state.events.subscribe();
    let mut transport = bootstrap::connect(&mut harness, &mut presence).await?;

    lifecycle::exercise(&mut harness, &mut transport).await?;
    control::exercise(&harness, &mut transport).await?;
    reconnect::exercise(&mut harness, transport, &mut presence).await?;
    Ok(())
}
