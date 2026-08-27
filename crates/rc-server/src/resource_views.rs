use crate::{AppState, now_ms};
use rusqlite::OptionalExtension;

const WORKSPACE_COLUMNS: &str = "w.id,w.name,wm.role,w.created_at";
const DEVICE_QUERY: &str = r#"
    SELECT d.id,d.workspace_id,w.name,d.name,d.hostname,d.platform,d.arch,d.version,
           d.capabilities,d.last_seen,d.created_at,wm.role,d.identity_public_key,
           d.transport_public_key,
           (SELECT count(*) FROM processes p
              WHERE p.device_id=d.id AND p.status IN ('starting','running'))
      FROM devices d
      JOIN workspaces w ON w.id=d.workspace_id
      JOIN workspace_members wm ON wm.workspace_id=d.workspace_id
     WHERE wm.user_id=?
     ORDER BY d.name
"#;
const PROCESS_COLUMNS: &str = r#"
    p.id,p.device_id,p.origin,p.status,p.terminal,p.exit_code,p.signal,p.error,
    p.created_by,u.name,p.created_at,p.started_at,p.completed_at
"#;

pub fn workspace_role(
    state: &AppState,
    user_id: &str,
    workspace_id: &str,
) -> rusqlite::Result<Option<String>> {
    state.db.with_connection(|db| {
        db.query_row(
            "SELECT role FROM workspace_members WHERE workspace_id=? AND user_id=?",
            rusqlite::params![workspace_id, user_id],
            |row| row.get(0),
        )
        .optional()
    })
}

pub fn workspace_json(
    state: &AppState,
    user_id: &str,
    workspace_id: &str,
) -> anyhow::Result<Option<serde_json::Value>> {
    let query = format!(
        "SELECT {WORKSPACE_COLUMNS} FROM workspaces w \
         JOIN workspace_members wm ON wm.workspace_id=w.id \
         WHERE w.id=? AND wm.user_id=?"
    );
    Ok(state.db.with_connection(|db| {
        db.query_row(
            &query,
            rusqlite::params![workspace_id, user_id],
            workspace_row_json,
        )
        .optional()
    })?)
}

pub fn workspaces_json(state: &AppState, user_id: &str) -> anyhow::Result<Vec<serde_json::Value>> {
    let query = format!(
        "SELECT {WORKSPACE_COLUMNS} FROM workspaces w \
         JOIN workspace_members wm ON wm.workspace_id=w.id \
         WHERE wm.user_id=? ORDER BY w.name"
    );
    Ok(state.db.with_connection(|db| {
        let mut statement = db.prepare(&query)?;
        statement
            .query_map([user_id], workspace_row_json)?
            .collect::<Result<Vec<_>, _>>()
    })?)
}

fn workspace_row_json(row: &rusqlite::Row<'_>) -> rusqlite::Result<serde_json::Value> {
    Ok(serde_json::json!({
        "id": row.get::<_, String>(0)?,
        "name": row.get::<_, String>(1)?,
        "role": row.get::<_, String>(2)?,
        "created_at": row.get::<_, i64>(3)?,
    }))
}

pub async fn devices_json(
    state: &AppState,
    user_id: &str,
) -> anyhow::Result<Vec<serde_json::Value>> {
    let rows = state.db.with_connection(|db| {
        let mut statement = db.prepare(DEVICE_QUERY)?;
        statement
            .query_map([user_id], DeviceRow::read)?
            .collect::<Result<Vec<_>, _>>()
    })?;
    let mut output = Vec::with_capacity(rows.len());
    for row in rows {
        let online = state.nodes.online(&row.id).await;
        output.push(row.into_json(online)?);
    }
    Ok(output)
}

pub async fn device_json(
    state: &AppState,
    user_id: &str,
    device_id: &str,
) -> anyhow::Result<Option<serde_json::Value>> {
    Ok(devices_json(state, user_id)
        .await?
        .into_iter()
        .find(|value| value.get("id").and_then(|value| value.as_str()) == Some(device_id)))
}

pub fn process_json(
    state: &AppState,
    user_id: &str,
    process_id: &str,
) -> anyhow::Result<Option<serde_json::Value>> {
    let query = format!(
        "SELECT {PROCESS_COLUMNS} FROM processes p \
         JOIN devices d ON d.id=p.device_id \
         JOIN workspace_members wm ON wm.workspace_id=d.workspace_id \
         LEFT JOIN users u ON u.id=p.created_by \
         WHERE p.id=? AND wm.user_id=?"
    );
    Ok(state.db.with_connection(|db| {
        db.query_row(
            &query,
            rusqlite::params![process_id, user_id],
            process_row_json,
        )
        .optional()
    })?)
}

pub fn processes_for_device(
    state: &AppState,
    user_id: &str,
    device_id: &str,
) -> anyhow::Result<Vec<serde_json::Value>> {
    let query = format!(
        "SELECT {PROCESS_COLUMNS} FROM processes p \
         JOIN devices d ON d.id=p.device_id \
         JOIN workspace_members wm ON wm.workspace_id=d.workspace_id \
         LEFT JOIN users u ON u.id=p.created_by \
         WHERE p.device_id=? AND wm.user_id=? \
         ORDER BY p.created_at DESC LIMIT 200"
    );
    Ok(state.db.with_connection(|db| {
        let mut statement = db.prepare(&query)?;
        statement
            .query_map(rusqlite::params![device_id, user_id], process_row_json)?
            .collect::<Result<Vec<_>, _>>()
    })?)
}

fn process_row_json(row: &rusqlite::Row<'_>) -> rusqlite::Result<serde_json::Value> {
    Ok(serde_json::json!({
        "id": row.get::<_, String>(0)?,
        "device_id": row.get::<_, String>(1)?,
        "origin": row.get::<_, String>(2)?,
        "status": row.get::<_, String>(3)?,
        "terminal": row.get::<_, i64>(4)? != 0,
        "exit_code": row.get::<_, Option<i64>>(5)?,
        "signal": row.get::<_, Option<String>>(6)?,
        "error": row.get::<_, Option<String>>(7)?,
        "created_by": row.get::<_, String>(8)?,
        "created_by_name": row.get::<_, Option<String>>(9)?,
        "created_at": row.get::<_, i64>(10)?,
        "started_at": row.get::<_, Option<i64>>(11)?,
        "completed_at": row.get::<_, Option<i64>>(12)?,
    }))
}

pub fn cleanup_expired(state: &AppState) -> rusqlite::Result<()> {
    state.db.with_connection(|db| {
        db.execute("DELETE FROM ceremonies WHERE expires_at<?", [now_ms()])?;
        Ok(())
    })
}

struct DeviceRow {
    id: String,
    workspace_id: String,
    workspace_name: String,
    name: String,
    hostname: String,
    platform: String,
    arch: String,
    version: String,
    capabilities: String,
    last_seen: Option<i64>,
    created_at: i64,
    role: String,
    identity_public_key: String,
    transport_public_key: String,
    active_processes: i64,
}

impl DeviceRow {
    fn read(row: &rusqlite::Row<'_>) -> rusqlite::Result<Self> {
        Ok(Self {
            id: row.get(0)?,
            workspace_id: row.get(1)?,
            workspace_name: row.get(2)?,
            name: row.get(3)?,
            hostname: row.get(4)?,
            platform: row.get(5)?,
            arch: row.get(6)?,
            version: row.get(7)?,
            capabilities: row.get(8)?,
            last_seen: row.get(9)?,
            created_at: row.get(10)?,
            role: row.get(11)?,
            identity_public_key: row.get(12)?,
            transport_public_key: row.get(13)?,
            active_processes: row.get(14)?,
        })
    }

    fn into_json(self, online: bool) -> anyhow::Result<serde_json::Value> {
        let capabilities = serde_json::from_str::<Vec<String>>(&self.capabilities)?;
        Ok(serde_json::json!({
            "id": self.id,
            "workspace_id": self.workspace_id,
            "workspace_name": self.workspace_name,
            "name": self.name,
            "hostname": self.hostname,
            "platform": self.platform,
            "arch": self.arch,
            "version": self.version,
            "capabilities": capabilities,
            "last_seen": self.last_seen,
            "created_at": self.created_at,
            "online": online,
            "active_processes": self.active_processes,
            "role": self.role,
            "identity_public_key": self.identity_public_key,
            "transport_public_key": self.transport_public_key,
        }))
    }
}
