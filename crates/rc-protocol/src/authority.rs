use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AuthorityCredential {
    pub id: String,
    pub public_key: String,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AuthorityMember {
    pub user_id: String,
    pub role: String,
    #[serde(default)]
    pub credentials: Vec<AuthorityCredential>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AuthorityApiKey {
    pub id: String,
    pub user_id: String,
    pub public_key: String,
    #[serde(default)]
    pub scopes: Vec<String>,
    #[serde(default, skip_serializing_if = "is_zero_i64")]
    pub expires_at: i64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AuthorityMcpGrant {
    pub id: String,
    pub user_id: String,
    pub hash: String,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AuthorityScheduleGrant {
    pub schedule_id: String,
    pub device_id: String,
    pub user_id: String,
    pub spec_hash: String,
    pub max_runtime_ms: u64,
    #[serde(default, skip_serializing_if = "is_zero_i64")]
    pub expires_at: i64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AuthorityDevice {
    pub id: String,
    pub identity_public_key: String,
    pub transport_public_key: String,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct McpGrantPayload {
    pub v: u32,
    pub id: String,
    pub user_id: String,
    pub client_id: String,
    pub client_name: String,
    #[serde(default)]
    pub device_ids: Vec<String>,
    #[serde(default)]
    pub scopes: Vec<String>,
    pub issued_at: i64,
    pub expires_at: i64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AuthoritySnapshot {
    pub v: u32,
    pub workspace_id: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub devices: Vec<AuthorityDevice>,
    #[serde(default)]
    pub members: Vec<AuthorityMember>,
    #[serde(default, rename = "apiKeys")]
    pub api_keys: Vec<AuthorityApiKey>,
    #[serde(default, rename = "mcpGrants", skip_serializing_if = "Vec::is_empty")]
    pub mcp_grants: Vec<AuthorityMcpGrant>,
    #[serde(
        default,
        rename = "scheduleGrants",
        skip_serializing_if = "Vec::is_empty"
    )]
    pub schedule_grants: Vec<AuthorityScheduleGrant>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ControlGrant {
    pub v: u32,
    pub client_id: String,
    pub user_id: String,
    pub signing_public_key: String,
    pub issued_at: i64,
    pub expires_at: i64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ControlProof {
    pub grant: String,
    pub credential_id: String,
    pub assertion: String,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct LockState {
    pub snapshot: String,
    pub origin: String,
    #[serde(rename = "rpId")]
    pub rp_id: String,
    pub generation: u64,
}

fn is_zero_i64(value: &i64) -> bool {
    *value == 0
}
