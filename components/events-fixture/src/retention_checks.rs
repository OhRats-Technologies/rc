use super::{device_event, workspace_event};
use crate::ohrats::rc_events::{append, types::LifecycleKind};

pub(super) fn verify_idempotency(workspace: &str) -> Result<(), String> {
    let retained = workspace_event(
        LifecycleKind::WorkspaceRenamed,
        workspace,
        "retained",
        Some("retained-retry"),
    );
    let first = append::append(&retained)?;
    if append::append(&retained)?.cursor != first.cursor {
        return Err("retained idempotency did not replay its event".into());
    }
    let conflict = workspace_event(
        LifecycleKind::WorkspaceRenamed,
        workspace,
        "conflict",
        Some("retained-retry"),
    );
    if append::append(&conflict).is_ok() {
        return Err("retained idempotency accepted a different event".into());
    }
    let newest = workspace_event(
        LifecycleKind::WorkspaceRenamed,
        workspace,
        "newest",
        Some("newest-retry"),
    );
    let newest = append::append(&newest)?;
    let final_event = append::append(&device_event(
        LifecycleKind::DeviceOnline,
        workspace,
        "device-retention",
    ))?;
    let retry = workspace_event(
        LifecycleKind::WorkspaceRenamed,
        workspace,
        "newest",
        Some("newest-retry"),
    );
    if append::append(&retry)?.cursor != newest.cursor || final_event.cursor <= newest.cursor {
        return Err("retained idempotency replay failed after pruning".into());
    }
    Ok(())
}
