use super::{CONTROL_CIPHERTEXT_LIMIT, ControlManager};
use crate::transport::webrtc::{complete_local_description, peer_connection};
use ::webrtc::{
    data_channel::{
        RTCDataChannel, data_channel_message::DataChannelMessage,
        data_channel_state::RTCDataChannelState,
    },
    peer_connection::{
        RTCPeerConnection, peer_connection_state::RTCPeerConnectionState,
        sdp::session_description::RTCSessionDescription,
    },
};
use rc_api_client::random_url_bytes;
use rc_protocol::{ControlTransportMessage, NodeToServer};
use std::{sync::Arc, time::Duration};
use tokio::sync::mpsc;

impl ControlManager {
    pub(super) async fn answer_webrtc(
        &self,
        request_id: String,
        session_id: String,
        sdp: String,
        ice_servers: Vec<rc_protocol::IceServer>,
        relay_only: bool,
    ) {
        if sdp.is_empty() || sdp.len() > 131_072 || ice_servers.len() > 8 {
            self.control_error(request_id, "invalid WebRTC offer");
            return;
        }
        if !self.has_session(&session_id) {
            self.control_error(request_id, "control session unavailable");
            return;
        }
        let peer = match peer_connection(&ice_servers, relay_only).await {
            Ok(value) => value,
            Err(_) => {
                self.control_error(request_id, "WebRTC unavailable");
                return;
            }
        };
        let transport_id = random_url_bytes(12);
        if !self
            .register_transport(&session_id, &transport_id, peer.clone())
            .await
        {
            let _ = peer.close().await;
            self.control_error(request_id, "control session unavailable");
            return;
        }
        self.configure_peer(&session_id, &transport_id, peer.clone());
        let description = match RTCSessionDescription::offer(sdp) {
            Ok(value) => value,
            Err(_) => {
                self.fail_webrtc(&session_id, &transport_id, peer, &request_id)
                    .await;
                return;
            }
        };
        if peer.set_remote_description(description).await.is_err() {
            self.fail_webrtc(&session_id, &transport_id, peer, &request_id)
                .await;
            return;
        }
        let answer = match peer.create_answer(None).await {
            Ok(value) => value,
            Err(_) => {
                self.fail_webrtc(&session_id, &transport_id, peer, &request_id)
                    .await;
                return;
            }
        };
        let answer = match complete_local_description(&peer, answer).await {
            Ok(value) => value,
            Err(_) => {
                self.fail_webrtc(&session_id, &transport_id, peer, &request_id)
                    .await;
                return;
            }
        };
        self.emit(NodeToServer::ControlWebrtcAnswer {
            request_id,
            session_id: session_id.clone(),
            sdp: answer,
        });
        let manager = self.clone();
        tokio::spawn(async move {
            tokio::time::sleep(Duration::from_secs(10)).await;
            if peer.connection_state() != RTCPeerConnectionState::Connected {
                manager
                    .clear_transport(&session_id, &transport_id, true)
                    .await;
            }
        });
    }

    fn configure_peer(&self, session_id: &str, transport_id: &str, peer: Arc<RTCPeerConnection>) {
        let manager = self.clone();
        let session = session_id.to_owned();
        let transport = transport_id.to_owned();
        peer.on_data_channel(Box::new(move |channel: Arc<RTCDataChannel>| {
            let manager = manager.clone();
            let session = session.clone();
            let transport = transport.clone();
            Box::pin(async move {
                if channel.label() != "rc-control" {
                    let _ = channel.close().await;
                    return;
                }
                let open_manager = manager.clone();
                let open_session = session.clone();
                let open_transport = transport.clone();
                let open_channel = channel.clone();
                channel.on_open(Box::new(move || {
                    let manager = open_manager.clone();
                    let session = open_session.clone();
                    let transport = open_transport.clone();
                    let channel = open_channel.clone();
                    Box::pin(async move {
                        manager.bind_channel(&session, &transport, channel);
                    })
                }));
                let message_manager = manager.clone();
                let message_session = session.clone();
                let message_channel = channel.clone();
                channel.on_message(Box::new(move |message: DataChannelMessage| {
                    let manager = message_manager.clone();
                    let session = message_session.clone();
                    let channel = message_channel.clone();
                    Box::pin(async move {
                        if !message.is_string || message.data.len() > CONTROL_CIPHERTEXT_LIMIT + 512
                        {
                            let _ = channel.close().await;
                            return;
                        }
                        let Ok(ControlTransportMessage::Frame {
                            session_id,
                            sequence,
                            ciphertext,
                        }) = serde_json::from_slice::<ControlTransportMessage>(&message.data)
                        else {
                            let _ = channel.close().await;
                            return;
                        };
                        if session_id != session
                            || manager
                                .receive_frame(&session_id, sequence, &ciphertext)
                                .is_err()
                        {
                            let _ = channel.close().await;
                        }
                    })
                }));
                let close_manager = manager.clone();
                let close_session = session.clone();
                let close_transport = transport.clone();
                channel.on_close(Box::new(move || {
                    let manager = close_manager.clone();
                    let session = close_session.clone();
                    let transport = close_transport.clone();
                    Box::pin(async move {
                        manager.transport_closed(&session, &transport).await;
                    })
                }));
                if channel.ready_state() == RTCDataChannelState::Open {
                    manager.bind_channel(&session, &transport, channel);
                }
            })
        }));
        let manager = self.clone();
        let session = session_id.to_owned();
        let transport = transport_id.to_owned();
        peer.on_peer_connection_state_change(Box::new(move |state| {
            let manager = manager.clone();
            let session = session.clone();
            let transport = transport.clone();
            Box::pin(async move {
                if matches!(
                    state,
                    RTCPeerConnectionState::Failed | RTCPeerConnectionState::Closed
                ) {
                    manager.transport_closed(&session, &transport).await;
                }
            })
        }));
    }

    fn bind_channel(&self, session_id: &str, transport_id: &str, channel: Arc<RTCDataChannel>) {
        let (tx, mut rx) = mpsc::unbounded_channel::<ControlTransportMessage>();
        {
            let mut sessions = self.0.sessions.lock();
            let Some(session) = sessions.get_mut(session_id) else {
                return;
            };
            if session.transport_id != transport_id {
                return;
            }
            session.sender = Some(tx);
        }
        let manager = self.clone();
        let session = session_id.to_owned();
        let transport = transport_id.to_owned();
        tokio::spawn(async move {
            while let Some(frame) = rx.recv().await {
                let Ok(text) = serde_json::to_string(&frame) else {
                    break;
                };
                if channel.send_text(text).await.is_err() {
                    break;
                }
            }
            manager.transport_closed(&session, &transport).await;
        });
    }

    async fn register_transport(
        &self,
        session_id: &str,
        transport_id: &str,
        peer: Arc<RTCPeerConnection>,
    ) -> bool {
        let previous = {
            let mut sessions = self.0.sessions.lock();
            let Some(session) = sessions.get_mut(session_id) else {
                return false;
            };
            session.sender = None;
            session.transport_id = transport_id.to_owned();
            session.peer.replace(peer)
        };
        if let Some(previous) = previous {
            let _ = previous.close().await;
        }
        true
    }
    async fn transport_closed(&self, session_id: &str, transport_id: &str) {
        let established = self
            .0
            .sessions
            .lock()
            .get(session_id)
            .filter(|session| session.transport_id == transport_id)
            .map(|session| session.sender.is_some());
        match established {
            Some(true) => self.close_session(session_id).await,
            Some(false) => self.clear_transport(session_id, transport_id, false).await,
            None => {}
        }
    }
    async fn clear_transport(&self, session_id: &str, transport_id: &str, close: bool) {
        let peer = {
            let mut sessions = self.0.sessions.lock();
            let Some(session) = sessions.get_mut(session_id) else {
                return;
            };
            if session.transport_id != transport_id {
                return;
            }
            session.sender = None;
            session.transport_id.clear();
            session.peer.take()
        };
        if close && let Some(peer) = peer {
            let _ = peer.close().await;
        }
    }
    async fn fail_webrtc(
        &self,
        session_id: &str,
        transport_id: &str,
        peer: Arc<RTCPeerConnection>,
        request_id: &str,
    ) {
        self.clear_transport(session_id, transport_id, false).await;
        let _ = peer.close().await;
        self.control_error(request_id, "WebRTC negotiation failed");
    }
}
