use crate::{AppState, now_ms};

pub(super) fn load(
    state: &AppState,
    workspace_id: &str,
) -> rusqlite::Result<(Vec<serde_json::Value>, Vec<serde_json::Value>)> {
    state.db.with_connection(|db| {
        let mut members = db.prepare(
            "SELECT wm.user_id,u.name,wm.role,wm.joined_at \
             FROM workspace_members wm \
             JOIN users u ON u.id=wm.user_id \
             WHERE wm.workspace_id=? ORDER BY wm.joined_at",
        )?;
        let members = members
            .query_map([workspace_id], |row| {
                Ok(serde_json::json!({
                    "user_id": row.get::<_, String>(0)?,
                    "name": row.get::<_, String>(1)?,
                    "role": row.get::<_, String>(2)?,
                    "joined_at": row.get::<_, i64>(3)?,
                }))
            })?
            .collect::<Result<Vec<_>, _>>()?;

        let mut invites = db.prepare(
            "SELECT id,role,created_at,expires_at \
             FROM workspace_invites \
             WHERE workspace_id=? AND used_at IS NULL AND expires_at>? \
             ORDER BY created_at DESC",
        )?;
        let invites = invites
            .query_map(rusqlite::params![workspace_id, now_ms()], |row| {
                Ok(serde_json::json!({
                    "id": row.get::<_, String>(0)?,
                    "role": row.get::<_, String>(1)?,
                    "created_at": row.get::<_, i64>(2)?,
                    "expires_at": row.get::<_, i64>(3)?,
                }))
            })?
            .collect::<Result<Vec<_>, _>>()?;
        Ok((members, invites))
    })
}
