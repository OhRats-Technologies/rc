use crate::{Database, now_ms};
use dashmap::DashMap;
use rusqlite::OptionalExtension;
use std::sync::Arc;

const RECENT_COMPLETION_TTL_MS: i64 = 5 * 60 * 1000;
const RECENT_COMPLETION_LIMIT: usize = 1_024;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ExecutionHistory {
    None,
    Metadata,
}

#[derive(Debug, Clone)]
pub struct ExecutionPolicy {
    history: ExecutionHistory,
    retention_ms: i64,
    recent: Arc<DashMap<String, RecentProcess>>,
}

#[derive(Debug, Clone)]
struct RecentProcess {
    workspace_id: String,
    expires_at: i64,
    value: serde_json::Value,
}

impl Default for ExecutionPolicy {
    fn default() -> Self {
        Self::new(ExecutionHistory::None, 168)
    }
}

impl ExecutionHistory {
    pub fn parse(value: &str) -> anyhow::Result<Self> {
        match value.trim().to_ascii_lowercase().as_str() {
            "" | "none" | "off" | "0" => Ok(Self::None),
            "metadata" => Ok(Self::Metadata),
            _ => anyhow::bail!("RC_EXECUTION_HISTORY must be none or metadata"),
        }
    }

    pub fn as_str(self) -> &'static str {
        match self {
            Self::None => "none",
            Self::Metadata => "metadata",
        }
    }
}

impl ExecutionPolicy {
    pub fn new(history: ExecutionHistory, retention_hours: u64) -> Self {
        let retention_ms = retention_hours
            .saturating_mul(60 * 60 * 1000)
            .min(i64::MAX as u64) as i64;
        Self {
            history,
            retention_ms,
            recent: Arc::new(DashMap::new()),
        }
    }

    pub fn history(&self) -> ExecutionHistory {
        self.history
    }

    pub fn persists_process_metadata(&self) -> bool {
        self.history == ExecutionHistory::Metadata
    }

    pub fn should_persist_event(&self, kind: &str) -> bool {
        !kind.starts_with("process.") || self.persists_process_metadata()
    }

    pub fn completed_process_filter(&self) -> &'static str {
        if self.persists_process_metadata() {
            ""
        } else {
            " AND p.status IN ('starting','running')"
        }
    }

    pub fn finalize(&self, db: &Database, process_id: &str) -> rusqlite::Result<()> {
        if self.persists_process_metadata() {
            self.cleanup_expired(db)
        } else {
            self.capture_completion(db, process_id)?;
            db.with_connection(|connection| {
                connection.execute(
                    "DELETE FROM processes WHERE id=? AND status IN ('exited','lost')",
                    [process_id],
                )?;
                Ok(())
            })
        }
    }

    pub fn recent_process(
        &self,
        db: &Database,
        user_id: &str,
        process_id: &str,
    ) -> rusqlite::Result<Option<serde_json::Value>> {
        self.cleanup_recent();
        let Some(record) = self.recent.get(process_id).map(|entry| entry.clone()) else {
            return Ok(None);
        };
        let allowed = db.with_connection(|connection| {
            connection
                .query_row(
                    "SELECT 1 FROM workspace_members WHERE workspace_id=? AND user_id=?",
                    rusqlite::params![record.workspace_id, user_id],
                    |_| Ok(()),
                )
                .optional()
                .map(|value| value.is_some())
        })?;
        Ok(allowed.then_some(record.value))
    }

    pub fn cleanup_startup(&self, db: &Database) -> rusqlite::Result<()> {
        if self.persists_process_metadata() {
            self.cleanup_expired(db)
        } else {
            db.with_connection_mut(|connection| {
                let transaction = connection.transaction()?;
                transaction.execute("DELETE FROM events WHERE kind LIKE 'process.%'", [])?;
                transaction.execute(
                    "DELETE FROM processes WHERE status IN ('exited','lost')",
                    [],
                )?;
                transaction.commit()
            })
        }
    }

    fn cleanup_expired(&self, db: &Database) -> rusqlite::Result<()> {
        let cutoff = now_ms().saturating_sub(self.retention_ms);
        db.with_connection_mut(|connection| {
            let transaction = connection.transaction()?;
            transaction.execute(
                "DELETE FROM events WHERE kind LIKE 'process.%' AND created_at<?",
                [cutoff],
            )?;
            transaction.execute(
                "DELETE FROM processes WHERE status IN ('exited','lost') AND completed_at<?",
                [cutoff],
            )?;
            transaction.commit()
        })
    }

    fn capture_completion(&self, db: &Database, process_id: &str) -> rusqlite::Result<()> {
        let record = db.with_connection(|connection| {
            connection
                .query_row(
                    "SELECT d.workspace_id,p.id,p.device_id,p.origin,p.status,p.terminal,p.exit_code,p.signal,p.error,p.created_by,u.name,p.created_at,p.started_at,p.completed_at \
                     FROM processes p JOIN devices d ON d.id=p.device_id \
                     LEFT JOIN users u ON u.id=p.created_by \
                     WHERE p.id=? AND p.status IN ('exited','lost')",
                    [process_id],
                    |row| {
                        Ok(RecentProcess {
                            workspace_id: row.get(0)?,
                            expires_at: now_ms() + RECENT_COMPLETION_TTL_MS,
                            value: serde_json::json!({
                                "id":row.get::<_,String>(1)?,
                                "device_id":row.get::<_,String>(2)?,
                                "origin":row.get::<_,String>(3)?,
                                "status":row.get::<_,String>(4)?,
                                "terminal":row.get::<_,i64>(5)? != 0,
                                "exit_code":row.get::<_,Option<i64>>(6)?,
                                "signal":row.get::<_,Option<String>>(7)?,
                                "error":row.get::<_,Option<String>>(8)?,
                                "created_by":row.get::<_,String>(9)?,
                                "created_by_name":row.get::<_,Option<String>>(10)?,
                                "created_at":row.get::<_,i64>(11)?,
                                "started_at":row.get::<_,Option<i64>>(12)?,
                                "completed_at":row.get::<_,Option<i64>>(13)?,
                                "ephemeral":true,
                            }),
                        })
                    },
                )
                .optional()
        })?;
        if let Some(record) = record {
            self.cleanup_recent();
            self.recent.insert(process_id.to_owned(), record);
            if self.recent.len() > RECENT_COMPLETION_LIMIT {
                let mut entries: Vec<_> = self
                    .recent
                    .iter()
                    .map(|entry| (entry.expires_at, entry.key().clone()))
                    .collect();
                entries.sort_by_key(|entry| entry.0);
                let excess = self.recent.len().saturating_sub(RECENT_COMPLETION_LIMIT);
                for (_, id) in entries.into_iter().take(excess) {
                    self.recent.remove(&id);
                }
            }
        }
        Ok(())
    }

    fn cleanup_recent(&self) {
        let now = now_ms();
        let expired: Vec<_> = self
            .recent
            .iter()
            .filter(|entry| entry.expires_at <= now)
            .map(|entry| entry.key().clone())
            .collect();
        for id in expired {
            self.recent.remove(&id);
        }
    }
}
