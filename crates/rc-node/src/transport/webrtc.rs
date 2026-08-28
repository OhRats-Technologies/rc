use rc_protocol::IceServer;
use std::sync::Arc;
use webrtc::{
    api::APIBuilder,
    ice_transport::ice_server::RTCIceServer,
    peer_connection::{
        RTCPeerConnection, configuration::RTCConfiguration,
        policy::ice_transport_policy::RTCIceTransportPolicy,
    },
};

pub async fn peer_connection(
    servers: &[IceServer],
    relay_only: bool,
) -> anyhow::Result<Arc<RTCPeerConnection>> {
    let configuration = peer_configuration(servers, relay_only);
    Ok(Arc::new(
        APIBuilder::new()
            .build()
            .new_peer_connection(configuration)
            .await?,
    ))
}

fn peer_configuration(servers: &[IceServer], relay_only: bool) -> RTCConfiguration {
    RTCConfiguration {
        ice_servers: servers
            .iter()
            .map(|server| RTCIceServer {
                urls: server.urls.clone(),
                username: server.username.clone(),
                credential: server.credential.clone(),
            })
            .collect(),
        ice_transport_policy: if relay_only {
            RTCIceTransportPolicy::Relay
        } else {
            RTCIceTransportPolicy::All
        },
        ..Default::default()
    }
}

pub async fn complete_local_description(
    peer: &RTCPeerConnection,
    description: webrtc::peer_connection::sdp::session_description::RTCSessionDescription,
) -> anyhow::Result<String> {
    let mut complete = peer.gathering_complete_promise().await;
    peer.set_local_description(description).await?;
    let _ = tokio::time::timeout(std::time::Duration::from_secs(15), complete.recv()).await;
    let sdp = peer
        .local_description()
        .await
        .map(|value| value.sdp)
        .ok_or_else(|| anyhow::anyhow!("missing local SDP"))?;
    anyhow::ensure!(
        sdp.contains("a=candidate:"),
        "ICE gathering produced no candidates"
    );
    Ok(sdp)
}

#[cfg(test)]
mod tests {
    use super::peer_configuration;
    use rc_protocol::IceServer;
    use webrtc::peer_connection::policy::ice_transport_policy::RTCIceTransportPolicy;

    #[test]
    fn relay_attempt_restricts_the_node_ice_agent() {
        let servers = vec![IceServer {
            urls: vec!["turn:turn.example.test:3478?transport=udp".into()],
            username: "user".into(),
            credential: "secret".into(),
        }];
        assert_eq!(
            peer_configuration(&servers, false).ice_transport_policy,
            RTCIceTransportPolicy::All
        );
        assert_eq!(
            peer_configuration(&servers, true).ice_transport_policy,
            RTCIceTransportPolicy::Relay
        );
    }
}
