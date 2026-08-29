use crate::ohrats::rc_api_credentials::types::{Credential, Kind, Lifetime, Scope};
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StoredCredential {
    pub id: String,
    pub user_id: String,
    pub kind: u8,
    pub name: String,
    pub public_key: String,
    pub scopes: Vec<u8>,
    pub created_at_ms: u64,
    pub expires_at_ms: u64,
    pub last_used_at_ms: Option<u64>,
    pub revoked_at_ms: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PendingCli {
    pub request_id: String,
    pub client_id: String,
    pub public_key: String,
    pub lifetime: Option<u8>,
    pub device_code_hash: Vec<u8>,
    pub user_code_hash: Vec<u8>,
    pub created_at_ms: u64,
    pub expires_at_ms: u64,
    pub approved_at_ms: Option<u64>,
    pub exchanged_at_ms: Option<u64>,
}

impl StoredCredential {
    pub fn output(&self) -> Credential {
        Credential {
            id: self.id.clone(),
            user_id: self.user_id.clone(),
            kind: match self.kind {
                1 => Kind::Cli,
                _ => Kind::Api,
            },
            name: self.name.clone(),
            public_key: self.public_key.clone(),
            scopes: self
                .scopes
                .iter()
                .map(|scope| match scope {
                    1 => Scope::Execute,
                    2 => Scope::ManageDevices,
                    3 => Scope::ManageWorkspaces,
                    _ => Scope::Read,
                })
                .collect(),
            created_at_ms: self.created_at_ms,
            expires_at_ms: self.expires_at_ms,
            last_used_at_ms: self.last_used_at_ms,
            revoked_at_ms: self.revoked_at_ms,
        }
    }
}

pub fn scope(value: Scope) -> u8 {
    match value {
        Scope::Read => 0,
        Scope::Execute => 1,
        Scope::ManageDevices => 2,
        Scope::ManageWorkspaces => 3,
    }
}

pub fn lifetime(value: Option<Lifetime>) -> Option<u8> {
    value.map(|value| match value {
        Lifetime::Never => 0,
        Lifetime::OneHour => 1,
        Lifetime::OneDay => 2,
        Lifetime::SevenDays => 3,
        Lifetime::ThirtyDays => 4,
        Lifetime::NinetyDays => 5,
        Lifetime::OneEightyDays => 6,
        Lifetime::OneYear => 7,
    })
}
