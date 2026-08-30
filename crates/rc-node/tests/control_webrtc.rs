#![cfg(unix)]

#[path = "control_webrtc/fixture.rs"]
mod fixture;
#[path = "support/mock_execution.rs"]
mod mock_execution;
#[path = "control_webrtc/peer.rs"]
mod peer;
mod policies;

use fixture::{open_control, recv_hosted, setup};
use peer::{
    assert_encrypted_process_output, assert_hosted_exit_without_plaintext, connect_peer,
    send_encrypted, wait_control_closed,
};
use rc_node::ExecutionManager;
use rc_protocol::{ControlMessage, NodeToServer, ServerToNode};

#[tokio::test]
async fn direct_webrtc_control_keeps_process_plaintext_off_hosted_channel() -> anyhow::Result<()> {
    let mut harness = setup()?;
    let session = open_control(&mut harness).await?;
    let mut peer = connect_peer(&harness.control, &mut harness.hosted, &session.id).await?;

    let command = ControlMessage::ProcessStart {
        id: "secret-process".into(),
        mode: rc_protocol::ExecutionMode::SystemShell {
            command: "printf 'phase34-secret'".into(),
        },
        cwd: None,
        environment: rc_protocol::EnvironmentSpec::default(),
        terminal: None,
    };
    send_encrypted(&peer.channel, &session.key, &session.id, 1, &command).await?;

    let permit_request = recv_hosted(&mut harness.hosted).await?;
    assert_eq!(
        permit_request,
        NodeToServer::ProcessStartRequest {
            id: "secret-process".into(),
            user_id: "user".into(),
        }
    );
    assert!(!serde_json::to_string(&permit_request)?.contains("phase34-secret"));
    harness
        .control
        .handle(
            "https://rc.example.test",
            ServerToNode::ProcessPermit {
                id: "secret-process".into(),
                user_id: "user".into(),
            },
        )
        .await;

    assert_encrypted_process_output(
        &mut peer.frames,
        &session.key,
        &session.id,
        "secret-process",
        b"phase34-secret",
    )
    .await?;
    assert_hosted_exit_without_plaintext(&mut harness.hosted, "secret-process", "phase34-secret")
        .await?;

    peer.channel.close().await?;
    wait_control_closed(&mut harness.hosted, &session.id).await?;
    assert!(!harness.control.has_session(&session.id));

    harness.processes.clear_secure_sink();
    harness.control.shutdown().await;
    harness.processes.shutdown();
    peer.peer.close().await?;
    let _ = std::fs::remove_dir_all(harness.state_dir);
    Ok(())
}
