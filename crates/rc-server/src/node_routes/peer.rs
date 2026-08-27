use crate::{AppState, NodeInbound};
use rc_protocol::NodeToServer;
use std::sync::Arc;
use webrtc::{
    data_channel::{
        RTCDataChannel, data_channel_message::DataChannelMessage,
        data_channel_state::RTCDataChannelState,
    },
    peer_connection::{RTCPeerConnection, peer_connection_state::RTCPeerConnectionState},
};

pub(super) fn configure(
    state: &AppState,
    device_id: &str,
    connection_id: &str,
    peer: Arc<RTCPeerConnection>,
) {
    let nodes = state.nodes.clone();
    let db = state.db.clone();
    let app = state.clone();
    let device = device_id.to_owned();
    let connection = connection_id.to_owned();
    peer.on_data_channel(Box::new(move |channel: Arc<RTCDataChannel>| {
        if channel.label() != "rc-node" {
            return Box::pin(async {});
        }
        let nodes = nodes.clone();
        let db = db.clone();
        let app = app.clone();
        let device = device.clone();
        let connection = connection.clone();
        Box::pin(async move {
            let open_nodes = nodes.clone();
            let open_device = device.clone();
            let open_connection = connection.clone();
            let open_channel = channel.clone();
            let open_app = app.clone();
            channel.on_open(Box::new(move || {
                let nodes = open_nodes.clone();
                let device = open_device.clone();
                let connection = open_connection.clone();
                let channel = open_channel.clone();
                let app = open_app.clone();
                Box::pin(async move {
                    if nodes.set_channel(&device, &connection, channel).await {
                        app.emit_device_presence(&device, true);
                    }
                })
            }));

            let close_nodes = nodes.clone();
            let close_app = app.clone();
            let close_device = device.clone();
            let close_connection = connection.clone();
            channel.on_close(Box::new(move || {
                let nodes = close_nodes.clone();
                let app = close_app.clone();
                let device = close_device.clone();
                let connection = close_connection.clone();
                Box::pin(async move {
                    if nodes.remove_if(&device, &connection).await {
                        app.release_device_sessions(&device);
                        app.emit_device_presence(&device, false);
                    }
                })
            }));

            let message_nodes = nodes.clone();
            let message_control = app.control.clone();
            let message_ssh = app.ssh.clone();
            let message_mcp = app.mcp.clone();
            let message_app = app.clone();
            let message_device = device.clone();
            channel.on_message(Box::new(move |message: DataChannelMessage| {
                let nodes = message_nodes.clone();
                let control = message_control.clone();
                let ssh = message_ssh.clone();
                let mcp = message_mcp.clone();
                let app = message_app.clone();
                let db = db.clone();
                let device = message_device.clone();
                Box::pin(async move {
                    if !message.is_string || message.data.len() > 1_048_576 {
                        return;
                    }
                    let Ok(value) = serde_json::from_slice::<NodeToServer>(&message.data) else {
                        return;
                    };
                    super::messages::apply(&app, &device, &value);
                    control.handle_node_message(&device, &value);
                    ssh.handle(&device, &value);
                    mcp.handle(&device, &value);
                    super::messages::bootstrap_lock_if_needed(&nodes, &db, &device, &value).await;
                    super::messages::permit_start_if_authorized(&nodes, &db, &device, &value).await;
                    nodes.publish(NodeInbound {
                        device_id: device,
                        message: value,
                    });
                })
            }));
            if channel.ready_state() == RTCDataChannelState::Open
                && nodes.set_channel(&device, &connection, channel).await
            {
                app.emit_device_presence(&device, true);
            }
        })
    }));

    let nodes = state.nodes.clone();
    let app = state.clone();
    let device = device_id.to_owned();
    let connection = connection_id.to_owned();
    let check_peer = peer.clone();
    peer.on_peer_connection_state_change(Box::new(move |status| {
        let nodes = nodes.clone();
        let app = app.clone();
        let device = device.clone();
        let connection = connection.clone();
        let peer = check_peer.clone();
        Box::pin(async move {
            let remove = match status {
                RTCPeerConnectionState::Failed | RTCPeerConnectionState::Closed => true,
                RTCPeerConnectionState::Disconnected => {
                    tokio::time::sleep(std::time::Duration::from_secs(15)).await;
                    peer.connection_state() == RTCPeerConnectionState::Disconnected
                }
                _ => false,
            };
            if remove && nodes.remove_if(&device, &connection).await {
                app.release_device_sessions(&device);
                app.emit_device_presence(&device, false);
            }
        })
    }));
}
