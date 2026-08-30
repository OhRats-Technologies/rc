use rc_protocol::ScheduleDefinition;
use std::sync::Arc;

pub trait ScheduleManager: Send + Sync {
    fn list(&self) -> Result<Vec<ScheduleDefinition>, String>;
    fn upsert(&self, schedule: ScheduleDefinition) -> Result<(), String>;
    fn remove(&self, id: &str) -> Result<bool, String>;
    fn set_enabled(&self, id: &str, enabled: bool) -> Result<bool, String>;
}

pub struct UnavailableScheduleManager;

impl ScheduleManager for UnavailableScheduleManager {
    fn list(&self) -> Result<Vec<ScheduleDefinition>, String> {
        Err("scheduler management is unavailable".into())
    }

    fn upsert(&self, _schedule: ScheduleDefinition) -> Result<(), String> {
        Err("scheduler management is unavailable".into())
    }

    fn remove(&self, _id: &str) -> Result<bool, String> {
        Err("scheduler management is unavailable".into())
    }

    fn set_enabled(&self, _id: &str, _enabled: bool) -> Result<bool, String> {
        Err("scheduler management is unavailable".into())
    }
}

pub fn unavailable_schedule_manager() -> Arc<dyn ScheduleManager> {
    Arc::new(UnavailableScheduleManager)
}
