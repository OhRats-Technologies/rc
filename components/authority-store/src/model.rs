use crate::ohrats::rc_authority::types::{
    ApiKey, ControlKey, Device, ExecutionGrant, Lock, Member, Passkey, Role, Snapshot,
};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

#[derive(Clone, Serialize, Deserialize)]
pub struct StoredState {
    pub snapshot: StoredSnapshot,
    pub generation: u64,
    pub hash: String,
    pub pending_invalidation: Option<u64>,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct StoredSnapshot {
    pub version: u32,
    pub workspace_id: String,
    pub devices: Vec<StoredDevice>,
    pub members: Vec<StoredMember>,
    pub api_keys: Vec<StoredApiKey>,
    pub active_execution_mcp_grants: Vec<StoredExecutionGrant>,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct StoredDevice {
    pub id: String,
    pub identity_public_key: Vec<u8>,
    pub transport_public_key: Vec<u8>,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct StoredMember {
    pub user_id: String,
    pub role: StoredRole,
    pub passkeys: Vec<StoredPasskey>,
    pub control_keys: Vec<StoredControlKey>,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct StoredPasskey {
    pub credential_id: String,
    pub public_key: Vec<u8>,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct StoredControlKey {
    pub id: String,
    pub public_key: Vec<u8>,
    pub authorized_by_passkey: String,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct StoredApiKey {
    pub id: String,
    pub user_id: String,
    pub public_key: Vec<u8>,
    pub scopes: Vec<String>,
    pub expires_at_ms: u64,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct StoredExecutionGrant {
    pub id: String,
    pub user_id: String,
    pub hash: String,
}

#[derive(Clone, Copy, Serialize, Deserialize)]
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

impl StoredRole {
    pub fn is_owner(self) -> bool {
        matches!(self, Self::Owner)
    }
}

impl From<&Snapshot> for StoredSnapshot {
    fn from(value: &Snapshot) -> Self {
        Self {
            version: value.version,
            workspace_id: value.workspace_id.clone(),
            devices: value.devices.iter().map(StoredDevice::from).collect(),
            members: value.members.iter().map(StoredMember::from).collect(),
            api_keys: value.api_keys.iter().map(StoredApiKey::from).collect(),
            active_execution_mcp_grants: value
                .active_execution_mcp_grants
                .iter()
                .map(StoredExecutionGrant::from)
                .collect(),
        }
    }
}

impl From<&Device> for StoredDevice {
    fn from(value: &Device) -> Self {
        Self {
            id: value.id.clone(),
            identity_public_key: value.identity_public_key.clone(),
            transport_public_key: value.transport_public_key.clone(),
        }
    }
}

impl From<&Member> for StoredMember {
    fn from(value: &Member) -> Self {
        Self {
            user_id: value.user_id.clone(),
            role: value.role.into(),
            passkeys: value.passkeys.iter().map(StoredPasskey::from).collect(),
            control_keys: value
                .control_keys
                .iter()
                .map(StoredControlKey::from)
                .collect(),
        }
    }
}

impl From<&Passkey> for StoredPasskey {
    fn from(value: &Passkey) -> Self {
        Self {
            credential_id: value.credential_id.clone(),
            public_key: value.public_key.clone(),
        }
    }
}

impl From<&ControlKey> for StoredControlKey {
    fn from(value: &ControlKey) -> Self {
        Self {
            id: value.id.clone(),
            public_key: value.public_key.clone(),
            authorized_by_passkey: value.authorized_by_passkey.clone(),
        }
    }
}

impl From<&ApiKey> for StoredApiKey {
    fn from(value: &ApiKey) -> Self {
        Self {
            id: value.id.clone(),
            user_id: value.user_id.clone(),
            public_key: value.public_key.clone(),
            scopes: value.scopes.clone(),
            expires_at_ms: value.expires_at_ms,
        }
    }
}

impl From<&ExecutionGrant> for StoredExecutionGrant {
    fn from(value: &ExecutionGrant) -> Self {
        Self {
            id: value.id.clone(),
            user_id: value.user_id.clone(),
            hash: value.hash.clone(),
        }
    }
}

impl From<&StoredSnapshot> for Snapshot {
    fn from(value: &StoredSnapshot) -> Self {
        Self {
            version: value.version,
            workspace_id: value.workspace_id.clone(),
            devices: value.devices.iter().map(Device::from).collect(),
            members: value.members.iter().map(Member::from).collect(),
            api_keys: value.api_keys.iter().map(ApiKey::from).collect(),
            active_execution_mcp_grants: value
                .active_execution_mcp_grants
                .iter()
                .map(ExecutionGrant::from)
                .collect(),
        }
    }
}

impl From<&StoredDevice> for Device {
    fn from(value: &StoredDevice) -> Self {
        Self {
            id: value.id.clone(),
            identity_public_key: value.identity_public_key.clone(),
            transport_public_key: value.transport_public_key.clone(),
        }
    }
}

impl From<&StoredMember> for Member {
    fn from(value: &StoredMember) -> Self {
        Self {
            user_id: value.user_id.clone(),
            role: value.role.into(),
            passkeys: value.passkeys.iter().map(Passkey::from).collect(),
            control_keys: value.control_keys.iter().map(ControlKey::from).collect(),
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

impl From<&StoredPasskey> for Passkey {
    fn from(value: &StoredPasskey) -> Self {
        Self {
            credential_id: value.credential_id.clone(),
            public_key: value.public_key.clone(),
        }
    }
}

impl From<&StoredControlKey> for ControlKey {
    fn from(value: &StoredControlKey) -> Self {
        Self {
            id: value.id.clone(),
            public_key: value.public_key.clone(),
            authorized_by_passkey: value.authorized_by_passkey.clone(),
        }
    }
}

impl From<&StoredApiKey> for ApiKey {
    fn from(value: &StoredApiKey) -> Self {
        Self {
            id: value.id.clone(),
            user_id: value.user_id.clone(),
            public_key: value.public_key.clone(),
            scopes: value.scopes.clone(),
            expires_at_ms: value.expires_at_ms,
        }
    }
}

impl From<&StoredExecutionGrant> for ExecutionGrant {
    fn from(value: &StoredExecutionGrant) -> Self {
        Self {
            id: value.id.clone(),
            user_id: value.user_id.clone(),
            hash: value.hash.clone(),
        }
    }
}

impl StoredState {
    pub fn lock(&self) -> Lock {
        Lock {
            snapshot: (&self.snapshot).into(),
            generation: self.generation,
            hash: self.hash.clone(),
        }
    }
}

pub fn snapshot_hash(value: &StoredSnapshot) -> String {
    Sha256::digest(serde_json::to_vec(value).expect("authority snapshot is serializable"))
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect()
}
