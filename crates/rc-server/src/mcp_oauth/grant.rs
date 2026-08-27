use super::client::{RequestRow, normalize_requested_scopes, request_row};
use crate::{
    AppState, MAX_FINITE_AUTH_LIFETIME_MS, MCP_DEFAULT_LIFETIME, UserIdentity, auth_lifetime,
    fresh_control_proof, hash, now_ms, opaque, verify_control_client_signature,
};
use rc_protocol::McpGrantPayload;
use serde::Serialize;
use sha2::{Digest, Sha256};
use uuid::Uuid;

#[derive(Debug, Serialize)]
pub struct PreparedGrant {
    pub grant: String,
    #[serde(rename = "signaturePayload")]
    pub signature_payload: String,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ApprovedGrant {
    pub redirect: String,
    pub grant_id: String,
    pub workspace_ids: Vec<String>,
    pub requires_sync: bool,
}

pub fn prepare_oauth_grant(
    state: &AppState,
    user: &UserIdentity,
    request_id: &str,
    device_ids: &[String],
    scopes: &[String],
    lifetime: Option<&str>,
) -> anyhow::Result<PreparedGrant> {
    let request = request_row(state, &user.id, request_id)?
        .ok_or_else(|| anyhow::anyhow!("MCP authorization expired"))?;
    let allowed = normalize_requested_scopes(&request.scope)?;
    if scopes.is_empty() || scopes.iter().any(|scope| !allowed.contains(scope)) {
        anyhow::bail!("scope was not requested by this MCP client");
    }
    let mut devices = device_ids.to_vec();
    devices.sort();
    devices.dedup();
    if devices.is_empty() || devices.len() > 100 {
        anyhow::bail!("select at least one device");
    }
    for device in &devices {
        let role = state
            .db
            .device_role(&user.id, device)?
            .ok_or_else(|| anyhow::anyhow!("device is not available to this account"))?;
        if scopes.iter().any(|scope| scope != "mcp:observe") && role != "owner" {
            anyhow::bail!("Terminal requires Owner access on every selected device");
        }
    }
    let issued = now_ms();
    let life =
        auth_lifetime(lifetime, MCP_DEFAULT_LIFETIME, true, issued).map_err(anyhow::Error::msg)?;
    let payload = McpGrantPayload {
        v: 1,
        id: Uuid::new_v4().to_string(),
        user_id: user.id.clone(),
        client_id: request.client_id.clone(),
        client_name: request.client_name.clone(),
        device_ids: devices,
        scopes: scopes.to_vec(),
        issued_at: issued,
        expires_at: life.expires_at,
    };
    let grant = serde_json::to_string(&payload)?;
    state.db.with_connection(|db| {
        db.execute(
            "UPDATE mcp_requests SET prepared_grant=? WHERE id=?",
            rusqlite::params![grant, request_id],
        )?;
        Ok(())
    })?;
    Ok(PreparedGrant {
        signature_payload: mcp_signature_payload(&grant),
        grant,
    })
}

pub fn approve_oauth_grant(
    state: &AppState,
    user: &UserIdentity,
    request_id: &str,
    control_client_id: &str,
    signature: &str,
) -> anyhow::Result<ApprovedGrant> {
    let request = request_row(state, &user.id, request_id)?
        .ok_or_else(|| anyhow::anyhow!("MCP authorization expired"))?;
    let grant = request
        .prepared_grant
        .clone()
        .ok_or_else(|| anyhow::anyhow!("MCP authorization expired"))?;
    let payload: McpGrantPayload = serde_json::from_str(&grant)?;
    validate_grant(&payload, &user.id)?;
    if !verify_control_client_signature(
        state,
        &user.id,
        control_client_id,
        &mcp_signature_payload(&grant),
        signature,
    )? {
        anyhow::bail!("invalid MCP grant signature");
    }
    let proof = fresh_control_proof(state, &user.id, control_client_id, 5 * 60_000)?
        .ok_or_else(|| anyhow::anyhow!("fresh passkey authorization required"))?;
    let code = state.db.with_connection_mut(|db| {
        let tx = db.transaction()?;
        tx.execute(
            "INSERT INTO mcp_grants(id,user_id,client_id,name,grant,grant_signature,client_control_id,credential_id,control_grant,control_assertion,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)",
            rusqlite::params![payload.id,user.id,payload.client_id,payload.client_name,grant,signature,control_client_id,proof.credential_id,proof.grant,proof.assertion,payload.issued_at,payload.expires_at],
        )?;
        let code = format!("mcp_code_{}", opaque(24));
        tx.execute(
            "INSERT INTO oauth_codes(code_hash,grant_id,redirect_uri,code_challenge,resource,expires_at) VALUES(?,?,?,?,?,?)",
            rusqlite::params![hash(&code),payload.id,request.redirect_uri,request.code_challenge,request.resource,now_ms()+5*60_000],
        )?;
        tx.execute("DELETE FROM mcp_requests WHERE id=?", [request_id])?;
        tx.commit()?;
        Ok(code)
    })?;
    approved_redirect(state, &request, &payload, code)
}

fn approved_redirect(
    state: &AppState,
    request: &RequestRow,
    payload: &McpGrantPayload,
    code: String,
) -> anyhow::Result<ApprovedGrant> {
    let mut redirect = url::Url::parse(&request.redirect_uri)?;
    redirect.query_pairs_mut().append_pair("code", &code);
    if !request.state.is_empty() {
        redirect
            .query_pairs_mut()
            .append_pair("state", &request.state);
    }
    redirect
        .query_pairs_mut()
        .append_pair("iss", state.config.public_url.trim_end_matches('/'));
    Ok(ApprovedGrant {
        redirect: redirect.into(),
        grant_id: payload.id.clone(),
        workspace_ids: grant_workspace_ids(state, &payload.device_ids)?,
        requires_sync: payload.scopes.iter().any(|scope| scope == "mcp:terminal"),
    })
}

fn validate_grant(payload: &McpGrantPayload, user: &str) -> anyhow::Result<()> {
    let now = now_ms();
    let invalid = payload.expires_at != 0
        && (payload.expires_at <= now
            || payload.expires_at <= payload.issued_at
            || payload.expires_at - payload.issued_at > MAX_FINITE_AUTH_LIFETIME_MS);
    if payload.v != 1 || payload.user_id != user || payload.issued_at > now + 60_000 || invalid {
        anyhow::bail!("invalid MCP grant");
    }
    Ok(())
}

fn mcp_signature_payload(grant: &str) -> String {
    let digest = Sha256::digest(grant.as_bytes());
    format!(
        "rc-mcp-grant-v1\n{}",
        digest
            .iter()
            .map(|byte| format!("{byte:02x}"))
            .collect::<String>()
    )
}

fn grant_workspace_ids(state: &AppState, devices: &[String]) -> anyhow::Result<Vec<String>> {
    use rusqlite::OptionalExtension;
    let mut out = Vec::new();
    for device in devices {
        if let Some(workspace) = state.db.with_connection(|db| {
            db.query_row(
                "SELECT workspace_id FROM devices WHERE id=?",
                [device],
                |row| row.get::<_, String>(0),
            )
            .optional()
        })? && !out.contains(&workspace)
        {
            out.push(workspace);
        }
    }
    out.sort();
    Ok(out)
}
