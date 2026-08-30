use super::ControlManager;
use rc_protocol::{ControlMessage, ScheduleDefinition, schedule_spec_hash};

impl ControlManager {
    pub(super) fn handle_schedule(
        &self,
        session_id: &str,
        user_id: &str,
        owner: bool,
        command: ControlMessage,
    ) -> anyhow::Result<()> {
        if !owner {
            anyhow::bail!("owner required");
        }
        let (request_id, result) = match command {
            ControlMessage::ScheduleList { request_id } => (request_id, self.0.schedules.list()),
            ControlMessage::ScheduleUpsert {
                request_id,
                schedule,
            } => {
                if schedule.created_by != user_id
                    || schedule_spec_hash(&schedule) != schedule.permit_hash
                {
                    anyhow::bail!("schedule authority metadata does not match definition");
                }
                crate::schedule_authority(
                    &self.0.state_dir,
                    &schedule.id,
                    &self.0.state.device_id,
                    &schedule.permit_hash,
                )?;
                let result = self.0.schedules.upsert(schedule).map(|_| Vec::new());
                (request_id, result)
            }
            ControlMessage::ScheduleRemove { request_id, id } => {
                (request_id, changed(self.0.schedules.remove(&id)))
            }
            ControlMessage::ScheduleSetEnabled {
                request_id,
                id,
                enabled,
            } => (
                request_id,
                changed(self.0.schedules.set_enabled(&id, enabled)),
            ),
            _ => anyhow::bail!("invalid schedule command"),
        };
        let (schedules, error) = match result {
            Ok(schedules) => (schedules, String::new()),
            Err(error) => (Vec::new(), error),
        };
        if !self.send_frame(
            session_id,
            &ControlMessage::ScheduleResult {
                request_id,
                schedules,
                error,
            },
        ) {
            anyhow::bail!("control session unavailable");
        }
        Ok(())
    }
}

fn changed(result: Result<bool, String>) -> Result<Vec<ScheduleDefinition>, String> {
    result.and_then(|changed| {
        changed
            .then_some(Vec::new())
            .ok_or_else(|| "schedule not found".into())
    })
}
