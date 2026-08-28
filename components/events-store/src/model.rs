use crate::ohrats::rc_events::types::{Detail, Event, LifecycleKind};
use serde::{Deserialize, Serialize};

#[derive(Serialize, Deserialize)]
pub struct StoredEvent {
    pub cursor: u64,
    pub kind: u8,
    pub occurred_at_ms: u64,
    pub actor_account_id: Option<String>,
    pub detail: StoredDetail,
}

#[derive(Serialize, Deserialize)]
pub struct StoredIdempotency {
    pub cursor: u64,
    pub request_sha256: Vec<u8>,
}

#[derive(Serialize, Deserialize)]
pub enum StoredDetail {
    Account(String, Option<String>),
    Workspace(String, Option<String>),
    Membership(String, String, Option<String>),
    Invitation(String, String),
    Device(String, String, Option<String>),
}

pub fn encode_kind(kind: LifecycleKind) -> u8 {
    kind as u8
}

pub fn decode_kind(value: u8) -> Result<LifecycleKind, String> {
    const KINDS: [LifecycleKind; 16] = [
        LifecycleKind::AccountCreated,
        LifecycleKind::AccountRenamed,
        LifecycleKind::AccountDeleted,
        LifecycleKind::WorkspaceCreated,
        LifecycleKind::WorkspaceRenamed,
        LifecycleKind::WorkspaceDeleted,
        LifecycleKind::WorkspaceMemberJoined,
        LifecycleKind::WorkspaceMemberRoleChanged,
        LifecycleKind::WorkspaceMemberLeft,
        LifecycleKind::WorkspaceInviteCreated,
        LifecycleKind::WorkspaceInviteRevoked,
        LifecycleKind::DeviceEnrolled,
        LifecycleKind::DeviceRenamed,
        LifecycleKind::DeviceOnline,
        LifecycleKind::DeviceOffline,
        LifecycleKind::DeviceRevoked,
    ];
    KINDS
        .get(value as usize)
        .copied()
        .ok_or_else(|| "invalid stored event kind".into())
}

impl StoredEvent {
    pub fn wire(self) -> Result<Event, String> {
        Ok(Event {
            cursor: self.cursor,
            kind: decode_kind(self.kind)?,
            occurred_at_ms: self.occurred_at_ms,
            actor_account_id: self.actor_account_id,
            detail: self.detail.wire(),
        })
    }
}

impl StoredDetail {
    pub fn wire(self) -> Detail {
        use crate::ohrats::rc_events::types::*;
        match self {
            Self::Account(account_id, display_name) => Detail::Account(AccountDetail {
                account_id,
                display_name,
            }),
            Self::Workspace(workspace_id, name) => {
                Detail::Workspace(WorkspaceDetail { workspace_id, name })
            }
            Self::Membership(workspace_id, account_id, role) => {
                Detail::Membership(MembershipDetail {
                    workspace_id,
                    account_id,
                    role,
                })
            }
            Self::Invitation(workspace_id, invitation_id) => Detail::Invitation(InvitationDetail {
                workspace_id,
                invitation_id,
            }),
            Self::Device(workspace_id, device_id, name) => Detail::Device(DeviceDetail {
                workspace_id,
                device_id,
                name,
            }),
        }
    }
}
