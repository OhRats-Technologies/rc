use crate::{AppState, Database, NodeHub, ProcessLifecycle};
use rc_protocol::{NodeToServer, ServerToNode};

pub(super) async fn bootstrap_lock_if_needed(
    nodes: &NodeHub,
    db: &Database,
    device: &str,
    message: &NodeToServer,
) {
    if let NodeToServer::Hello { hello } = message
        && hello.lock_hash.is_empty()
    {
        match crate::bootstrap_snapshot_for_device(db, device) {
            Ok(Some(snapshot)) => {
                let _ = nodes
                    .send(device, &ServerToNode::LockBootstrap { snapshot })
                    .await;
            }
            Ok(None) => {}
            Err(error) => tracing::warn!(%device, %error, "failed to build RC Lock bootstrap"),
        }
    }
}

pub(super) async fn permit_start_if_authorized(
    nodes: &NodeHub,
    db: &Database,
    device: &str,
    message: &NodeToServer,
) {
    if let NodeToServer::ProcessStartRequest { id, user_id } = message {
        match db.direct_process_permit(device, id, user_id) {
            Ok(true) => {
                let _ = nodes
                    .send(
                        device,
                        &ServerToNode::ProcessPermit {
                            id: id.clone(),
                            user_id: user_id.clone(),
                        },
                    )
                    .await;
            }
            Ok(false) => {}
            Err(error) => {
                tracing::warn!(%device, %error, "failed to authorize process start")
            }
        }
    }
}

pub(super) fn apply(state: &AppState, device_id: &str, message: &NodeToServer) {
    let result: anyhow::Result<()> = (|| {
        match message {
            NodeToServer::Hello { hello } => apply_hello(state, device_id, hello)?,
            NodeToServer::ProcessSync { ids } => apply_sync(state, device_id, ids)?,
            NodeToServer::ProcessStarted { id } => apply_started(state, device_id, id)?,
            NodeToServer::LockState { hash, generation } => {
                state.db.mark_lock_state(device_id, hash, *generation)?;
            }
            NodeToServer::ProcessExit {
                id,
                exit_code,
                signal,
                error,
            } => apply_exit(state, device_id, id, *exit_code, signal, error)?,
            NodeToServer::UpdateResult { ok, version, error } => {
                apply_update(state, device_id, *ok, version, error)?;
            }
            _ => {}
        }
        Ok(())
    })();
    if let Err(error) = result {
        tracing::warn!(%device_id, %error, "failed to apply Node message");
    }
}

fn apply_hello(
    state: &AppState,
    device_id: &str,
    hello: &rc_protocol::NodeHello,
) -> anyhow::Result<()> {
    if let Some(touch) = state.db.touch_node(device_id, hello)?
        && touch.version_changed
    {
        emit(
            state,
            "device.updated",
            &touch.workspace_id,
            None,
            device_id,
            serde_json::json!({"version":hello.version}),
        )?;
    }
    Ok(())
}

fn apply_sync(state: &AppState, device_id: &str, ids: &[String]) -> anyhow::Result<()> {
    for process in state.db.mark_process_sync(device_id, ids)? {
        emit_process(
            state,
            "process.lost",
            device_id,
            &process,
            serde_json::json!({
                "processId":process.id,
                "error":"Node reconnected without this process"
            }),
        )?;
        state.execution.finalize(&state.db, &process.id)?;
    }
    Ok(())
}

fn apply_started(state: &AppState, device_id: &str, id: &str) -> anyhow::Result<()> {
    if let Some(process) = state.db.mark_process_started(device_id, id)? {
        emit_process(
            state,
            "process.started",
            device_id,
            &process,
            serde_json::json!({"processId":process.id}),
        )?;
    }
    Ok(())
}

fn apply_exit(
    state: &AppState,
    device_id: &str,
    id: &str,
    exit_code: i32,
    signal: &str,
    error: &str,
) -> anyhow::Result<()> {
    if let Some(process) =
        state
            .db
            .mark_process_exit(device_id, id, exit_code, signal, &bounded_error(error))?
    {
        emit_process(
            state,
            "process.exited",
            device_id,
            &process,
            serde_json::json!({
                "processId":process.id,
                "exitCode":exit_code,
                "signal":signal,
                "error":bounded_error(error)
            }),
        )?;
        state.execution.finalize(&state.db, &process.id)?;
    }
    Ok(())
}

fn bounded_error(value: &str) -> String {
    value.chars().take(240).collect()
}

fn apply_update(
    state: &AppState,
    device_id: &str,
    ok: bool,
    version: &str,
    error: &str,
) -> anyhow::Result<()> {
    if let Some(workspace) = state.db.device_workspace(device_id)? {
        emit(
            state,
            if ok {
                "node.update.complete"
            } else {
                "node.update.error"
            },
            &workspace,
            None,
            device_id,
            serde_json::json!({"version":version,"error":error}),
        )?;
    }
    Ok(())
}

fn emit_process(
    state: &AppState,
    kind: &str,
    device_id: &str,
    process: &ProcessLifecycle,
    detail: serde_json::Value,
) -> anyhow::Result<()> {
    emit(
        state,
        kind,
        &process.workspace_id,
        Some(&process.user_id),
        device_id,
        detail,
    )
}

fn emit(
    state: &AppState,
    kind: &str,
    workspace_id: &str,
    user_id: Option<&str>,
    device_id: &str,
    detail: serde_json::Value,
) -> anyhow::Result<()> {
    state.events.emit(
        &state.db,
        kind,
        Some(workspace_id),
        user_id,
        Some(device_id),
        detail,
    )?;
    Ok(())
}
