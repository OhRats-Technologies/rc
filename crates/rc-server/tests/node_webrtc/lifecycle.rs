use super::{fixture::Harness, support};
use rc_node::ServerTransport;
use rc_protocol::{NodeToServer, ServerToNode};
use std::time::Duration;
use tokio::time::timeout;
use uuid::Uuid;

pub(super) async fn exercise(
    harness: &mut Harness,
    transport: &mut ServerTransport,
) -> anyhow::Result<()> {
    transport
        .send(&NodeToServer::ProcessStartRequest {
            id: harness.process_id.clone(),
            user_id: harness.user_id.clone(),
        })
        .await?;
    assert_eq!(
        timeout(Duration::from_secs(3), transport.recv())
            .await
            .map_err(|_| anyhow::anyhow!("timed out waiting for process permit"))?,
        Some(ServerToNode::ProcessPermit {
            id: harness.process_id.clone(),
            user_id: harness.user_id.clone(),
        })
    );
    transport
        .send(&NodeToServer::ProcessStartRequest {
            id: harness.process_id.clone(),
            user_id: "wrong-user".into(),
        })
        .await?;
    assert!(
        timeout(Duration::from_millis(150), transport.recv())
            .await
            .is_err(),
        "a process permit was issued for the wrong user"
    );

    let lifecycle_id = Uuid::new_v4().to_string();
    harness.seed.execute(
        "INSERT INTO processes(id,device_id,origin,status,terminal,created_by,created_at) VALUES(?,?,'browser','starting',1,?,?)",
        rusqlite::params![lifecycle_id, harness.device_id, harness.user_id, rc_server::now_ms()],
    )?;
    let mut lifecycle = harness.state.events.subscribe();
    transport
        .send(&NodeToServer::ProcessStarted {
            id: lifecycle_id.clone(),
        })
        .await?;
    let started =
        support::assert_event(&mut lifecycle, "process.started", &harness.device_id).await?;
    assert_eq!(started.process_id.as_deref(), Some(lifecycle_id.as_str()));
    assert!(started.audit);
    assert_eq!(started.detail["processId"], lifecycle_id);
    transport
        .send(&NodeToServer::ProcessStarted {
            id: lifecycle_id.clone(),
        })
        .await?;
    support::assert_no_event(&mut lifecycle).await?;

    transport
        .send(&NodeToServer::ProcessExit {
            id: lifecycle_id.clone(),
            exit_code: 17,
            signal: String::new(),
        })
        .await?;
    let exited =
        support::assert_event(&mut lifecycle, "process.exited", &harness.device_id).await?;
    assert_eq!(exited.process_id.as_deref(), Some(lifecycle_id.as_str()));
    assert_eq!(exited.detail["exitCode"], 17);
    transport
        .send(&NodeToServer::ProcessExit {
            id: lifecycle_id,
            exit_code: 17,
            signal: String::new(),
        })
        .await?;
    support::assert_no_event(&mut lifecycle).await?;

    let lost_id = Uuid::new_v4().to_string();
    harness.seed.execute(
        "INSERT INTO processes(id,device_id,origin,status,terminal,created_by,created_at) VALUES(?,?,'browser','running',1,?,?)",
        rusqlite::params![lost_id, harness.device_id, harness.user_id, rc_server::now_ms()],
    )?;
    transport
        .send(&NodeToServer::ProcessSync { ids: Vec::new() })
        .await?;
    let mut lost = Vec::new();
    while lost.len() < 2 {
        let event =
            support::assert_event(&mut lifecycle, "process.lost", &harness.device_id).await?;
        lost.push(event.process_id.unwrap_or_default());
    }
    lost.sort();
    let mut expected = vec![harness.process_id.clone(), lost_id];
    expected.sort();
    assert_eq!(lost, expected);
    transport
        .send(&NodeToServer::ProcessSync { ids: Vec::new() })
        .await?;
    support::assert_no_event(&mut lifecycle).await?;
    Ok(())
}
