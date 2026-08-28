use super::{Database, now_ms};
use rusqlite::{OptionalExtension, params};

impl Database {
    pub fn mark_process_sync(
        &self,
        device_id: &str,
        active: &[String],
    ) -> rusqlite::Result<Vec<ProcessLifecycle>> {
        let mut db = self.lock()?;
        let tx = db.transaction()?;
        let rows = {
            let mut statement = tx.prepare(
                "SELECT p.id,d.workspace_id,p.created_by FROM processes p JOIN devices d ON d.id=p.device_id WHERE p.device_id=? AND p.status IN ('starting','running')",
            )?;
            statement
                .query_map([device_id], process_lifecycle_row)?
                .collect::<Result<Vec<_>, _>>()?
        };
        let now = now_ms();
        let mut lost = Vec::new();
        for row in rows {
            if !active.iter().any(|value| value == &row.id)
                && tx.execute(
                    "UPDATE processes SET status='lost',error='Node reconnected without this process',completed_at=? WHERE id=? AND status IN ('starting','running')",
                    params![now, row.id],
                )? == 1
            {
                lost.push(row);
            }
        }
        tx.commit()?;
        Ok(lost)
    }

    pub fn mark_process_started(
        &self,
        device_id: &str,
        id: &str,
    ) -> rusqlite::Result<Option<ProcessLifecycle>> {
        self.transition_process(
            device_id,
            id,
            "UPDATE processes SET status='running',started_at=COALESCE(started_at,?) WHERE id=? AND device_id=? AND status='starting'",
            params![now_ms(), id, device_id],
        )
    }

    pub fn direct_process_permit(
        &self,
        device_id: &str,
        id: &str,
        user_id: &str,
    ) -> rusqlite::Result<bool> {
        self.lock()?
            .query_row(
                "SELECT 1 FROM processes WHERE id=? AND device_id=? AND created_by=? AND status='starting' AND origin IN ('browser','cli','api')",
                params![id, device_id, user_id],
                |_| Ok(()),
            )
            .optional()
            .map(|value| value.is_some())
    }

    pub fn device_role(&self, user_id: &str, device_id: &str) -> rusqlite::Result<Option<String>> {
        self.lock()?
            .query_row(
                "SELECT wm.role FROM devices d JOIN workspace_members wm ON wm.workspace_id=d.workspace_id WHERE d.id=? AND wm.user_id=?",
                params![device_id, user_id],
                |row| row.get(0),
            )
            .optional()
    }

    pub fn mark_process_exit(
        &self,
        device_id: &str,
        id: &str,
        exit_code: i32,
        signal: &str,
    ) -> rusqlite::Result<Option<ProcessLifecycle>> {
        self.transition_process(
            device_id,
            id,
            "UPDATE processes SET status='exited',exit_code=?,signal=?,completed_at=? WHERE id=? AND device_id=? AND status IN ('starting','running')",
            params![exit_code, signal, now_ms(), id, device_id],
        )
    }

    pub fn mark_process_lost(
        &self,
        device_id: &str,
        id: &str,
        reason: &str,
    ) -> rusqlite::Result<Option<ProcessLifecycle>> {
        self.transition_process(
            device_id,
            id,
            "UPDATE processes SET status='lost',error=?,completed_at=? WHERE id=? AND device_id=? AND status IN ('starting','running')",
            params![reason, now_ms(), id, device_id],
        )
    }

    fn transition_process<P: rusqlite::Params>(
        &self,
        device_id: &str,
        id: &str,
        sql: &str,
        params: P,
    ) -> rusqlite::Result<Option<ProcessLifecycle>> {
        let mut db = self.lock()?;
        let tx = db.transaction()?;
        let row = tx
            .query_row(
                "SELECT p.id,d.workspace_id,p.created_by FROM processes p JOIN devices d ON d.id=p.device_id WHERE p.id=? AND p.device_id=?",
                params![id, device_id],
                process_lifecycle_row,
            )
            .optional()?;
        let Some(row) = row else {
            tx.commit()?;
            return Ok(None);
        };
        let changed = tx.execute(sql, params)? == 1;
        tx.commit()?;
        Ok(changed.then_some(row))
    }
}

fn process_lifecycle_row(row: &rusqlite::Row<'_>) -> rusqlite::Result<ProcessLifecycle> {
    Ok(ProcessLifecycle {
        id: row.get(0)?,
        workspace_id: row.get(1)?,
        user_id: row.get(2)?,
    })
}

#[derive(Debug, Clone)]
pub struct ProcessLifecycle {
    pub id: String,
    pub workspace_id: String,
    pub user_id: String,
}
