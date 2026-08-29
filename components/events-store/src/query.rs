use crate::{
    model::StoredEvent,
    ohrats::rc_events::types::{Authorization, Batch, Detail, Event, Filter},
    storage, validate,
};

const MAX_PAGE: u32 = 500;

pub fn run(auth: Authorization, after: u64, limit: u32, filter: Filter) -> Result<Batch, String> {
    validate::id(&auth.requester_account_id, "requester account id")?;
    if limit == 0 || limit > MAX_PAGE {
        return Err("event page limit is outside supported bounds".into());
    }
    for workspace in &auth.workspace_ids {
        validate::id(workspace, "authorized workspace id")?;
    }
    if let Some(device) = filter.device_id.as_deref() {
        validate::id(device, "device filter")?;
    }
    if let Some(account) = filter.account_id.as_deref() {
        validate::id(account, "account filter")?;
    }
    if let Some(workspace) = filter.workspace_id.as_deref() {
        validate::id(workspace, "workspace filter")?;
        if !auth
            .workspace_ids
            .iter()
            .any(|allowed| allowed == workspace)
        {
            return Err("workspace is not authorized".into());
        }
    }
    let entries = storage::scan(storage::EVENTS)?;
    let oldest = entries
        .first()
        .and_then(|e| decode(e).ok())
        .map_or(0, |e| e.cursor);
    let reset_required = after != 0 && oldest != 0 && after.saturating_add(1) < oldest;
    let mut events = Vec::new();
    let mut next_cursor = after;
    for entry in entries {
        let event = decode(&entry)?.wire()?;
        if event.cursor <= after {
            continue;
        }
        next_cursor = event.cursor;
        if authorized(&event, &auth) && matches(&event, &filter) {
            events.push(event);
            if events.len() == limit as usize {
                break;
            }
        }
    }
    Ok(Batch {
        events,
        next_cursor,
        reset_required,
    })
}

fn decode(entry: &crate::ohrats::rc_storage::types::Entry) -> Result<StoredEvent, String> {
    serde_json::from_slice(&entry.value).map_err(|error| error.to_string())
}

fn authorized(event: &Event, auth: &Authorization) -> bool {
    match &event.detail {
        Detail::Account(value) => {
            auth.include_own_account_events && value.account_id == auth.requester_account_id
        }
        Detail::Workspace(value) => auth.workspace_ids.contains(&value.workspace_id),
        Detail::Membership(value) => auth.workspace_ids.contains(&value.workspace_id),
        Detail::Invitation(value) => auth.workspace_ids.contains(&value.workspace_id),
        Detail::Device(value) => auth.workspace_ids.contains(&value.workspace_id),
    }
}

fn matches(event: &Event, filter: &Filter) -> bool {
    if !filter.kinds.is_empty() && !filter.kinds.contains(&event.kind) {
        return false;
    }
    let (workspace, device, account) = match &event.detail {
        Detail::Account(v) => (None, None, Some(v.account_id.as_str())),
        Detail::Workspace(v) => (Some(v.workspace_id.as_str()), None, None),
        Detail::Membership(v) => (
            Some(v.workspace_id.as_str()),
            None,
            Some(v.account_id.as_str()),
        ),
        Detail::Invitation(v) => (Some(v.workspace_id.as_str()), None, None),
        Detail::Device(v) => (
            Some(v.workspace_id.as_str()),
            Some(v.device_id.as_str()),
            None,
        ),
    };
    filter
        .workspace_id
        .as_deref()
        .is_none_or(|v| Some(v) == workspace)
        && filter
            .device_id
            .as_deref()
            .is_none_or(|v| Some(v) == device)
        && filter
            .account_id
            .as_deref()
            .is_none_or(|v| Some(v) == account)
}
