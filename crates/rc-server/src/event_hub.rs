use crate::{Database, ExecutionPolicy, now_ms};
use std::sync::Arc;
use tokio::sync::broadcast;

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RcEvent {
    pub kind: String,
    pub workspace_id: Option<String>,
    pub user_id: Option<String>,
    pub device_id: Option<String>,
    pub process_id: Option<String>,
    pub audit: bool,
    pub detail: serde_json::Value,
    pub at: i64,
}

#[derive(Clone)]
pub struct EventHub {
    sender: Arc<broadcast::Sender<RcEvent>>,
    execution: ExecutionPolicy,
}

impl Default for EventHub {
    fn default() -> Self {
        Self::new(ExecutionPolicy::default())
    }
}

impl EventHub {
    pub fn new(execution: ExecutionPolicy) -> Self {
        let (sender, _) = broadcast::channel(1024);
        Self {
            sender: Arc::new(sender),
            execution,
        }
    }

    pub fn subscribe(&self) -> broadcast::Receiver<RcEvent> {
        self.sender.subscribe()
    }

    pub fn cleanup_transient(db: &Database) -> rusqlite::Result<()> {
        db.with_connection(|connection| {
            connection.execute(
                "DELETE FROM events WHERE kind IN ('device.online','device.offline','rc.connected')",
                [],
            )?;
            Ok(())
        })
    }

    pub fn emit(
        &self,
        db: &Database,
        kind: &str,
        workspace_id: Option<&str>,
        user_id: Option<&str>,
        device_id: Option<&str>,
        detail: serde_json::Value,
    ) -> rusqlite::Result<()> {
        let at = now_ms();
        let process_id = detail
            .get("processId")
            .and_then(serde_json::Value::as_str)
            .map(str::to_owned);
        let audit = !is_transient(kind) && self.execution.should_persist_event(kind);
        if audit {
            db.with_connection(|connection| {
                connection.execute(
                    "INSERT INTO events(workspace_id,user_id,device_id,kind,detail,created_at) VALUES(?,?,?,?,?,?)",
                    rusqlite::params![workspace_id,user_id,device_id,kind,serde_json::to_string(&detail).unwrap_or_else(|_|"{}".into()),at],
                )?;
                Ok(())
            })?;
        }
        let _ = self.sender.send(RcEvent {
            kind: kind.to_owned(),
            workspace_id: workspace_id.map(str::to_owned),
            user_id: user_id.map(str::to_owned),
            device_id: device_id.map(str::to_owned),
            process_id,
            audit,
            detail,
            at,
        });
        Ok(())
    }
}

fn is_transient(kind: &str) -> bool {
    matches!(kind, "device.online" | "device.offline" | "rc.connected")
}
