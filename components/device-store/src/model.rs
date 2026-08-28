use crate::ohrats::rc_devices::types::{Device, Presence, Tombstone};
use serde::{Deserialize, Serialize};

pub const DEVICES: &str = "devices";
pub const IDENTITIES: &str = "device-identities";
pub const TOKENS: &str = "enrollment-tokens";
pub const TOMBSTONES: &str = "device-tombstones";
pub const PRESENCES: &str = "device-presence";

#[derive(Clone, Serialize, Deserialize)]
pub struct StoredDevice {
    pub id: String,
    pub workspace_id: String,
    pub name: String,
    pub hostname: String,
    pub platform: String,
    pub arch: String,
    pub identity_public_key: String,
    pub transport_public_key: String,
    pub version: String,
    pub capabilities: Vec<String>,
    pub last_seen_at_ms: Option<u64>,
    pub created_at_ms: u64,
}

#[derive(Serialize, Deserialize)]
pub struct StoredToken {
    pub workspace_id: String,
    pub created_by: String,
    pub created_at_ms: u64,
    pub expires_at_ms: u64,
    pub device_id: Option<String>,
    pub identity_public_key: Option<String>,
    pub used_at_ms: Option<u64>,
}

#[derive(Clone, Serialize, Deserialize)]
pub struct StoredTombstone {
    pub device_id: String,
    pub identity_public_key: String,
    pub revoked_at_ms: u64,
}

#[derive(Serialize, Deserialize)]
pub struct StoredPresence {
    pub device_id: String,
    pub last_seen_at_ms: u64,
    pub lease_expires_at_ms: u64,
    pub lock_hash: String,
    pub lock_generation: u64,
    pub rendezvous: Option<String>,
}

impl From<StoredDevice> for Device {
    fn from(value: StoredDevice) -> Self {
        Self {
            id: value.id,
            workspace_id: value.workspace_id,
            name: value.name,
            hostname: value.hostname,
            platform: value.platform,
            arch: value.arch,
            identity_public_key: value.identity_public_key,
            transport_public_key: value.transport_public_key,
            version: value.version,
            capabilities: value.capabilities,
            last_seen_at_ms: value.last_seen_at_ms,
            created_at_ms: value.created_at_ms,
        }
    }
}

impl From<StoredTombstone> for Tombstone {
    fn from(value: StoredTombstone) -> Self {
        Self {
            device_id: value.device_id,
            identity_public_key: value.identity_public_key,
            revoked_at_ms: value.revoked_at_ms,
        }
    }
}

impl StoredPresence {
    pub fn view(self, now_ms: u64) -> Presence {
        Presence {
            device_id: self.device_id,
            online: self.lease_expires_at_ms > now_ms,
            last_seen_at_ms: Some(self.last_seen_at_ms),
            lease_expires_at_ms: (self.lease_expires_at_ms > now_ms)
                .then_some(self.lease_expires_at_ms),
            lock_hash: self.lock_hash,
            lock_generation: self.lock_generation,
            rendezvous: (self.lease_expires_at_ms > now_ms)
                .then_some(self.rendezvous)
                .flatten(),
        }
    }
}

pub fn encode(value: &impl Serialize) -> Result<Vec<u8>, String> {
    serde_json::to_vec(value).map_err(|error| error.to_string())
}

pub fn decode<T: for<'a> Deserialize<'a>>(value: &[u8]) -> Result<T, String> {
    serde_json::from_slice(value).map_err(|error| error.to_string())
}
