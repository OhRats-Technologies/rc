use super::fixture::Harness;
use rc_node::ServerTransport;
use rc_protocol::{NodeToServer, ServerToNode};
use std::time::Duration;
use tokio::time::timeout;

pub(super) async fn exercise(
    harness: &Harness,
    transport: &mut ServerTransport,
) -> anyhow::Result<()> {
    let challenge_hub = harness.state.control.clone();
    let challenge_user = harness.user_id.clone();
    let challenge_device = harness.device_id.clone();
    let challenge_task = tokio::spawn(async move {
        challenge_hub
            .challenge(&challenge_user, &challenge_device)
            .await
    });
    let request_id = match timeout(Duration::from_secs(3), transport.recv()).await? {
        Some(ServerToNode::ControlChallenge { request_id }) => request_id,
        other => anyhow::bail!("unexpected coordinator challenge request: {other:?}"),
    };
    transport
        .send(&NodeToServer::ControlChallenge {
            request_id,
            challenge: "coordinator-challenge".into(),
        })
        .await?;
    assert_eq!(challenge_task.await??, "coordinator-challenge");

    let open_hub = harness.state.control.clone();
    let open_user = harness.user_id.clone();
    let open_device = harness.device_id.clone();
    let open_task = tokio::spawn(async move {
        open_hub
            .open(
                &open_user,
                &open_device,
                "test-client",
                "coordinator-challenge",
                "client-transport-key",
                "client-signature",
                None,
            )
            .await
    });
    let request_id = match timeout(Duration::from_secs(3), transport.recv()).await? {
        Some(ServerToNode::ControlOpen {
            request_id,
            user_id,
            client_id,
            grant,
            ..
        }) => {
            assert_eq!(user_id, harness.user_id);
            assert_eq!(client_id, "test-client");
            assert!(grant.is_empty());
            request_id
        }
        other => anyhow::bail!("unexpected coordinator open request: {other:?}"),
    };
    transport
        .send(&NodeToServer::ControlReady {
            request_id,
            session_id: "control-session".into(),
            transport_public_key: "node-transport".into(),
            ephemeral_public_key: "node-ephemeral".into(),
            signature: "node-signature".into(),
        })
        .await?;
    let ready = open_task.await??;
    assert_eq!(ready.session_id, "control-session");
    assert!(ready.ice_servers.is_empty());
    assert!(harness.state.control.has_session("control-session"));

    let webrtc_hub = harness.state.control.clone();
    let webrtc_user = harness.user_id.clone();
    let webrtc_device = harness.device_id.clone();
    let webrtc_task = tokio::spawn(async move {
        webrtc_hub
            .webrtc(
                &webrtc_user,
                Some("test-client"),
                "control-session",
                &webrtc_device,
                "client-offer",
                false,
            )
            .await
    });
    let request_id = match timeout(Duration::from_secs(3), transport.recv()).await? {
        Some(ServerToNode::ControlWebrtcOffer {
            request_id,
            session_id,
            sdp,
            ice_servers,
            relay_only,
        }) => {
            assert_eq!(session_id, "control-session");
            assert_eq!(sdp, "client-offer");
            assert!(!relay_only);
            assert!(ice_servers.is_empty());
            request_id
        }
        other => anyhow::bail!("unexpected coordinator WebRTC request: {other:?}"),
    };
    transport
        .send(&NodeToServer::ControlWebrtcAnswer {
            request_id,
            session_id: "control-session".into(),
            sdp: "node-answer".into(),
        })
        .await?;
    assert_eq!(webrtc_task.await??, "node-answer");
    Ok(())
}
