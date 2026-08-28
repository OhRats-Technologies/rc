use rc_protocol::IceServer;
use std::sync::Arc;
use webrtc::{
    api::APIBuilder,
    ice_transport::ice_server::RTCIceServer,
    peer_connection::{RTCPeerConnection, configuration::RTCConfiguration},
};

pub async fn peer_connection(servers: &[IceServer]) -> anyhow::Result<Arc<RTCPeerConnection>> {
    let configuration = RTCConfiguration {
        ice_servers: servers
            .iter()
            .map(|server| RTCIceServer {
                urls: server.urls.clone(),
                username: server.username.clone(),
                credential: server.credential.clone(),
            })
            .collect(),
        ..Default::default()
    };
    Ok(Arc::new(
        APIBuilder::new()
            .build()
            .new_peer_connection(configuration)
            .await?,
    ))
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
