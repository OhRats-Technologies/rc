use crate::ohrats::rc_workspaces::types::{Access, Invitation, Membership, Role, Workspace};
use serde::{Deserialize, Serialize};

#[derive(Clone, Serialize, Deserialize)]
pub struct StoredWorkspace {
    pub id: String,
    pub name: String,
    pub personal_for: Option<String>,
    pub created_at_ms: u64,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct StoredMembership {
    pub workspace_id: String,
    pub user_id: String,
    pub role: StoredRole,
    pub created_at_ms: u64,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct StoredInvitation {
    pub id: String,
    pub workspace_id: String,
    pub role: StoredRole,
    pub created_by: String,
    pub created_at_ms: u64,
    pub expires_at_ms: u64,
}

#[derive(Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum StoredRole {
    Viewer,
    Operator,
    Owner,
}

impl From<Role> for StoredRole {
    fn from(value: Role) -> Self {
        match value {
            Role::Viewer => Self::Viewer,
            Role::Operator => Self::Operator,
            Role::Owner => Self::Owner,
        }
    }
}
impl From<StoredRole> for Role {
    fn from(value: StoredRole) -> Self {
        match value {
            StoredRole::Viewer => Self::Viewer,
            StoredRole::Operator => Self::Operator,
            StoredRole::Owner => Self::Owner,
        }
    }
}
impl From<StoredWorkspace> for Workspace {
    fn from(v: StoredWorkspace) -> Self {
        Self {
            id: v.id,
            name: v.name,
            personal_for: v.personal_for,
            created_at_ms: v.created_at_ms,
        }
    }
}
impl From<StoredMembership> for Membership {
    fn from(v: StoredMembership) -> Self {
        Self {
            workspace_id: v.workspace_id,
            user_id: v.user_id,
            role: v.role.into(),
            created_at_ms: v.created_at_ms,
        }
    }
}
impl From<StoredInvitation> for Invitation {
    fn from(v: StoredInvitation) -> Self {
        Self {
            id: v.id,
            workspace_id: v.workspace_id,
            role: v.role.into(),
            created_by: v.created_by,
            created_at_ms: v.created_at_ms,
            expires_at_ms: v.expires_at_ms,
        }
    }
}

pub fn access(workspace: StoredWorkspace, membership: StoredMembership) -> Access {
    Access {
        workspace: workspace.into(),
        role: membership.role.into(),
    }
}
