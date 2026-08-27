use crate::AppState;
use axum::{
    extract::{
        State, WebSocketUpgrade,
        ws::{Message, WebSocket},
    },
    response::{IntoResponse, Response},
};
use futures_util::{SinkExt, StreamExt};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt},
    net::TcpStream,
};

pub(super) async fn tunnel(State(state): State<AppState>, upgrade: WebSocketUpgrade) -> Response {
    upgrade
        .on_upgrade(move |socket| tunnel_socket(state, socket))
        .into_response()
}

async fn tunnel_socket(state: AppState, socket: WebSocket) {
    let Ok(stream) = TcpStream::connect(("127.0.0.1", state.config.ssh_daemon_port)).await else {
        return;
    };
    let (mut tcp_read, mut tcp_write) = stream.into_split();
    let (mut ws_send, mut ws_recv) = socket.split();
    let to_tcp = async {
        while let Some(message) = ws_recv.next().await {
            match message {
                Ok(Message::Binary(data)) => {
                    if tcp_write.write_all(&data).await.is_err() {
                        break;
                    }
                }
                Ok(Message::Close(_)) | Err(_) => break,
                _ => {}
            }
        }
        let _ = tcp_write.shutdown().await;
    };
    let to_ws = async {
        let mut buffer = vec![0u8; 32 * 1024];
        loop {
            match tcp_read.read(&mut buffer).await {
                Ok(0) | Err(_) => break,
                Ok(count) => {
                    if ws_send
                        .send(Message::Binary(buffer[..count].to_vec().into()))
                        .await
                        .is_err()
                    {
                        break;
                    }
                }
            }
        }
        let _ = ws_send.close().await;
    };
    tokio::join!(to_tcp, to_ws);
}
