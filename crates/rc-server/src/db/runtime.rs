use super::Database;
use crate::ExecutionHistory;

impl Database {
    /// Applies the configured process-retention policy and reconciles process
    /// rows that could not have survived the previous server process.
    pub fn configure_execution_history(&self, history: ExecutionHistory) -> rusqlite::Result<()> {
        let value = match history {
            ExecutionHistory::None => "none",
            ExecutionHistory::Metadata => "metadata",
        };
        self.with_connection_mut(|connection| {
            let transaction = connection.transaction()?;
            transaction.execute(
                "INSERT INTO runtime_settings(key,value) VALUES('execution_history',?) \
                 ON CONFLICT(key) DO UPDATE SET value=excluded.value",
                [value],
            )?;
            transaction.execute(
                "UPDATE processes \
                 SET status='lost',error=COALESCE(error,'RC server restarted'), \
                     completed_at=COALESCE(completed_at,?) \
                 WHERE status IN ('starting','running')",
                [crate::now_ms()],
            )?;
            if history == ExecutionHistory::None {
                transaction.execute(
                    "DELETE FROM processes WHERE status IN ('exited','lost')",
                    [],
                )?;
            }
            transaction.commit()
        })
    }
}
