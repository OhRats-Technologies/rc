use super::super::encode;
use anyhow::{Result, bail};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use ed25519_dalek::Signer;
use rc_api_client::{ApiClient, Credential, Device};
use rc_protocol::ScheduleDefinition;
use serde::Deserialize;
use std::time::Duration;

pub fn pop(credential: &Credential) -> Result<&rc_api_client::PopKey> {
    match credential {
        Credential::Pop(key) => Ok(key),
        Credential::Bearer(_) => bail!("schedule management requires passkey-backed `rc login`"),
    }
}

pub async fn create_permit(
    api: &ApiClient,
    credential: &Credential,
    device: &Device,
    schedule: &ScheduleDefinition,
) -> Result<()> {
    let client_id = pop(credential)?.id.clone();
    let path = permit_path(device, &schedule.id);
    let result = api.put::<_, serde_json::Value>(&path, &serde_json::json!({
        "clientId":client_id, "deviceId":device.id, "specHash":schedule.permit_hash,
        "maxRuntimeMs":schedule.max_runtime_ms.unwrap_or_default(), "expiresAt":schedule.expires_at_ms.unwrap_or_default()
    })).await;
    result.map(|_| ()).map_err(|error| {
        anyhow::anyhow!(
            "{error}; durable schedule creation requires `rc login` within the last five minutes"
        )
    })
}

pub async fn remove_permit(
    api: &ApiClient,
    device: &Device,
    id: &str,
    client_id: &str,
) -> Result<()> {
    api.delete_json::<_, serde_json::Value>(
        &permit_path(device, id),
        &serde_json::json!({"clientId":client_id}),
    )
    .await?;
    Ok(())
}

#[derive(Deserialize)]
struct AuthorityStatus {
    hash: String,
    parents: Vec<AuthorityParent>,
    devices: usize,
    synced: usize,
}
#[derive(Deserialize)]
struct AuthorityParent {
    hash: String,
    generation: u64,
}

pub async fn sync_authority(
    api: &ApiClient,
    credential: &Credential,
    workspace: &str,
) -> Result<()> {
    let key = pop(credential)?;
    let path = format!("/api/v1/workspaces/{}/authority", encode(workspace));
    let state: AuthorityStatus = api.get(&path).await?;
    if state.devices == state.synced {
        return Ok(());
    }
    let transitions: Vec<_> = state.parents.iter().map(|parent| {
        let payload = format!("rc-authority-v3\n{}\n{}\n{}", parent.generation, parent.hash, state.hash);
        serde_json::json!({"fromHash":parent.hash,"generation":parent.generation,"signature":URL_SAFE_NO_PAD.encode(key.signing_key().sign(payload.as_bytes()).to_bytes())})
    }).collect();
    if transitions.is_empty() {
        bail!("RC Lock state is unavailable for synchronization");
    }
    let _: serde_json::Value = api
        .post(
            &format!("{path}/sync"),
            &serde_json::json!({"clientId":key.id,"transitions":transitions}),
        )
        .await?;
    for _ in 0..20 {
        tokio::time::sleep(Duration::from_millis(200)).await;
        let next: AuthorityStatus = api.get(&path).await?;
        if next.devices == next.synced {
            return Ok(());
        }
    }
    bail!("RC Lock synchronization timed out")
}

pub async fn current_user(api: &ApiClient) -> Result<String> {
    #[derive(Deserialize)]
    struct Me {
        user: User,
    }
    #[derive(Deserialize)]
    struct User {
        id: String,
    }
    Ok(api.get::<Me>("/api/v1/me").await?.user.id)
}

fn permit_path(device: &Device, id: &str) -> String {
    format!(
        "/api/v1/workspaces/{}/schedule-grants/{}",
        encode(&device.workspace_id),
        encode(id)
    )
}
