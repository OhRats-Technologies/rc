use crate::{AppState, now_ms};

pub(super) fn passkeys(state: &AppState, user_id: &str) -> anyhow::Result<Vec<serde_json::Value>> {
    Ok(state.db.with_connection(|db| {
        let mut statement = db.prepare(
            "SELECT id,created_at,last_used FROM passkeys WHERE user_id=? ORDER BY created_at",
        )?;
        statement
            .query_map([user_id], |row| {
                Ok(serde_json::json!({
                    "id":row.get::<_,String>(0)?,
                    "created_at":row.get::<_,i64>(1)?,
                    "last_used":row.get::<_,Option<i64>>(2)?,
                }))
            })?
            .collect::<Result<Vec<_>, _>>()
    })?)
}

pub(super) fn api_keys(state: &AppState, user_id: &str) -> anyhow::Result<Vec<serde_json::Value>> {
    let rows = state.db.with_connection(|db| {
        let mut statement = db.prepare(
            "SELECT id,name,scopes,created_at,expires_at,last_used FROM clients WHERE user_id=? AND kind='api' ORDER BY created_at DESC",
        )?;
        statement
            .query_map([user_id], |row| {
                Ok((
                    row.get::<_, String>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, String>(2)?,
                    row.get::<_, i64>(3)?,
                    row.get::<_, i64>(4)?,
                    row.get::<_, Option<i64>>(5)?,
                ))
            })?
            .collect::<Result<Vec<_>, _>>()
    })?;
    rows.into_iter()
        .map(|(id, name, scopes, created_at, expires_at, last_used)| {
            Ok(serde_json::json!({
                "id":id,
                "name":name,
                "scopes":serde_json::from_str::<Vec<String>>(&scopes)?,
                "created_at":created_at,
                "expires_at":expires_at,
                "last_used":last_used,
            }))
        })
        .collect()
}

pub(super) fn mcp_grants(
    state: &AppState,
    user_id: &str,
) -> anyhow::Result<Vec<crate::page_html::McpPageGrant>> {
    let rows = state.db.with_connection(|db| {
        let mut statement = db.prepare(
            "SELECT id,name,grant,expires_at,last_used FROM mcp_grants WHERE user_id=? AND revoked_at IS NULL AND (expires_at=0 OR expires_at>?) ORDER BY created_at DESC",
        )?;
        statement
            .query_map(rusqlite::params![user_id, now_ms()], |row| {
                Ok((
                    row.get::<_, String>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, String>(2)?,
                    row.get::<_, i64>(3)?,
                    row.get::<_, Option<i64>>(4)?,
                ))
            })?
            .collect::<Result<Vec<_>, _>>()
    })?;
    rows.into_iter()
        .map(|(id, name, grant, expires_at, last_used)| {
            let payload = serde_json::from_str::<rc_protocol::McpGrantPayload>(&grant)?;
            Ok(crate::page_html::McpPageGrant {
                id,
                name,
                scopes: payload.scopes.join(", "),
                expires_at,
                last_used,
                device_count: payload.device_ids.len(),
            })
        })
        .collect()
}

pub(super) fn workspace_access(
    state: &AppState,
    workspace_id: &str,
) -> anyhow::Result<(Vec<serde_json::Value>, Vec<serde_json::Value>)> {
    Ok(state.db.with_connection(|db| {
        let mut member_query = db.prepare(
            "SELECT wm.user_id,u.name,wm.role,wm.joined_at FROM workspace_members wm JOIN users u ON u.id=wm.user_id WHERE wm.workspace_id=? ORDER BY wm.joined_at",
        )?;
        let members = member_query
            .query_map([workspace_id], |row| {
                Ok(serde_json::json!({
                    "user_id":row.get::<_,String>(0)?,
                    "name":row.get::<_,String>(1)?,
                    "role":row.get::<_,String>(2)?,
                    "joined_at":row.get::<_,i64>(3)?,
                }))
            })?
            .collect::<Result<Vec<_>, _>>()?;
        let mut invite_query = db.prepare(
            "SELECT id,role,created_at,expires_at FROM workspace_invites WHERE workspace_id=? AND used_at IS NULL AND expires_at>? ORDER BY created_at DESC",
        )?;
        let invites = invite_query
            .query_map(rusqlite::params![workspace_id, now_ms()], |row| {
                Ok(serde_json::json!({
                    "id":row.get::<_,String>(0)?,
                    "role":row.get::<_,String>(1)?,
                    "created_at":row.get::<_,i64>(2)?,
                    "expires_at":row.get::<_,i64>(3)?,
                }))
            })?
            .collect::<Result<Vec<_>, _>>()?;
        Ok((members, invites))
    })?)
}

pub(super) fn activity(
    state: &AppState,
    workspace_id: &str,
) -> anyhow::Result<Vec<serde_json::Value>> {
    let rows = state.db.with_connection(|db| {
        let mut statement = db.prepare(
            "SELECT id,kind,detail,device_id,created_at FROM events WHERE workspace_id=? ORDER BY created_at DESC LIMIT 100",
        )?;
        statement
            .query_map([workspace_id], |row| {
                Ok((
                    row.get::<_, i64>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, String>(2)?,
                    row.get::<_, Option<String>>(3)?,
                    row.get::<_, i64>(4)?,
                ))
            })?
            .collect::<Result<Vec<_>, _>>()
    })?;
    rows.into_iter()
        .map(|(id, kind, detail, device_id, created_at)| {
            Ok(serde_json::json!({
                "id":id,
                "kind":kind,
                "detail":serde_json::from_str::<serde_json::Value>(&detail)?,
                "device_id":device_id,
                "created_at":created_at,
            }))
        })
        .collect()
}
