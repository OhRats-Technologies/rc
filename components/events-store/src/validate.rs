use crate::ohrats::rc_events::types::{AppendRequest, Detail, LifecycleKind};

const MAX_ID: usize = 128;
const MAX_NAME: usize = 120;
const MAX_IDEMPOTENCY: usize = 128;

pub fn request(value: &AppendRequest) -> Result<(), String> {
    if value.occurred_at_ms == 0 {
        return Err("event timestamp is required".into());
    }
    if let Some(actor) = value.actor_account_id.as_deref() {
        id(actor, "actor account id")?;
    }
    if let Some(key) = value.idempotency_key.as_deref()
        && (key.is_empty() || key.len() > MAX_IDEMPOTENCY || key.chars().any(char::is_control))
    {
        return Err("invalid idempotency key".into());
    }
    match (&value.kind, &value.detail) {
        (
            LifecycleKind::AccountCreated
            | LifecycleKind::AccountRenamed
            | LifecycleKind::AccountDeleted,
            Detail::Account(v),
        ) => {
            id(&v.account_id, "account id")?;
            name(v.display_name.as_deref())?;
        }
        (
            LifecycleKind::WorkspaceCreated
            | LifecycleKind::WorkspaceRenamed
            | LifecycleKind::WorkspaceDeleted,
            Detail::Workspace(v),
        ) => {
            id(&v.workspace_id, "workspace id")?;
            name(v.name.as_deref())?;
        }
        (
            LifecycleKind::WorkspaceMemberJoined
            | LifecycleKind::WorkspaceMemberRoleChanged
            | LifecycleKind::WorkspaceMemberLeft,
            Detail::Membership(v),
        ) => {
            id(&v.workspace_id, "workspace id")?;
            id(&v.account_id, "account id")?;
            name(v.role.as_deref())?;
        }
        (
            LifecycleKind::WorkspaceInviteCreated | LifecycleKind::WorkspaceInviteRevoked,
            Detail::Invitation(v),
        ) => {
            id(&v.workspace_id, "workspace id")?;
            id(&v.invitation_id, "invitation id")?;
        }
        (
            LifecycleKind::DeviceEnrolled
            | LifecycleKind::DeviceRenamed
            | LifecycleKind::DeviceOnline
            | LifecycleKind::DeviceOffline
            | LifecycleKind::DeviceRevoked,
            Detail::Device(v),
        ) => {
            id(&v.workspace_id, "workspace id")?;
            id(&v.device_id, "device id")?;
            name(v.name.as_deref())?;
        }
        _ => return Err("event kind does not match structured detail".into()),
    }
    Ok(())
}

pub fn id(value: &str, label: &str) -> Result<(), String> {
    if !value.is_empty()
        && value.len() <= MAX_ID
        && value
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || matches!(b, b'.' | b'-' | b'_' | b':'))
    {
        Ok(())
    } else {
        Err(format!("invalid {label}"))
    }
}

fn name(value: Option<&str>) -> Result<(), String> {
    if value.is_none_or(|v| {
        !v.trim().is_empty() && v.len() <= MAX_NAME && !v.chars().any(char::is_control)
    }) {
        Ok(())
    } else {
        Err("invalid bounded event label".into())
    }
}

#[cfg(test)]
mod tests {
    use super::request;
    use crate::ohrats::rc_events::types::{AccountDetail, AppendRequest, Detail, LifecycleKind};

    fn valid() -> AppendRequest {
        AppendRequest {
            kind: LifecycleKind::AccountCreated,
            occurred_at_ms: 1,
            actor_account_id: None,
            detail: Detail::Account(AccountDetail {
                account_id: "account-1".into(),
                display_name: Some("Ada".into()),
            }),
            idempotency_key: Some("retry-1".into()),
        }
    }

    #[test]
    fn accepts_bounded_typed_lifecycle_event() {
        assert!(request(&valid()).is_ok());
    }

    #[test]
    fn rejects_kind_detail_mismatch_and_oversize_label() {
        let mut value = valid();
        value.kind = LifecycleKind::DeviceOnline;
        assert!(request(&value).is_err());
        value = valid();
        if let Detail::Account(detail) = &mut value.detail {
            detail.display_name = Some("x".repeat(121));
        }
        assert!(request(&value).is_err());
    }
}
