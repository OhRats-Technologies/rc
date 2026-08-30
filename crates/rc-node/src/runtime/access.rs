use super::NodeRuntime;
use crate::{ExecutionManager, ScheduleManager};
use std::sync::Arc;

impl NodeRuntime {
    pub fn set_schedule_manager(&mut self, schedules: Arc<dyn ScheduleManager>) {
        self.schedules = schedules;
    }

    pub fn manager(&self) -> &dyn ExecutionManager {
        self.manager.as_ref()
    }

    pub fn manager_arc(&self) -> Arc<dyn ExecutionManager> {
        self.manager.clone()
    }

    pub fn context(&self) -> &rc_context::Context {
        &self.services
    }

    pub fn mesh(&self) -> &rc_mesh::RouteBroker {
        &self.mesh
    }
}
