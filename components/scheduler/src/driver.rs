use crate::{
    component::{
        exports::ohrats::rc_scheduler::driver::{TickResult, Trigger},
        ohrats::rc_scheduler::types::{Definition, MisfirePolicy, Occurrence},
    },
    engine::{self, Cursor, CursorStore, Schedule},
    model::StoredCursor,
};
use std::collections::HashMap;

struct DurableCursors;

impl CursorStore for DurableCursors {
    fn get(&mut self, id: &str) -> Result<Option<Cursor>, String> {
        Ok(crate::storage::cursor(id)?.map(|value| Cursor {
            checked_at_ms: value.checked_at_ms,
            last_occurrence_id: value.last_occurrence_id,
        }))
    }

    fn put(&mut self, id: &str, value: &Cursor) -> Result<(), String> {
        crate::storage::put_cursor(
            id,
            &StoredCursor {
                checked_at_ms: value.checked_at_ms,
                last_occurrence_id: value.last_occurrence_id.clone(),
            },
        )
    }
}

pub fn tick(now_ms: u64, recovering: bool, active_ids: Vec<String>) -> Result<TickResult, String> {
    let definitions = crate::storage::list()?;
    let schedules = definitions.iter().map(view).collect();
    let fired = engine::tick(
        &mut DurableCursors,
        schedules,
        now_ms,
        recovering,
        active_ids,
    )?;
    let mut definitions: HashMap<_, _> = definitions
        .into_iter()
        .map(|value| (value.id.clone(), value))
        .collect();
    let triggers = fired
        .triggers
        .into_iter()
        .map(|value| {
            Ok(Trigger {
                schedule: definitions
                    .remove(&value.schedule_id)
                    .ok_or("scheduler definition disappeared during tick")?,
                occurrence: Occurrence {
                    id: value.occurrence.id,
                    scheduled_at_ms: value.occurrence.scheduled_at_ms,
                },
            })
        })
        .collect::<Result<Vec<_>, String>>()?;
    Ok(TickResult {
        triggers,
        next_wake_unix_ms: fired.next_wake_ms,
    })
}

fn view(value: &Definition) -> Schedule {
    Schedule {
        id: value.id.clone(),
        cron: value.cron.clone(),
        timezone: value.timezone.clone(),
        enabled: value.enabled,
        skip_misfire: matches!(value.misfire, MisfirePolicy::Skip),
        created_at_ms: value.created_at_ms,
        expires_at_ms: value.expires_at_ms,
    }
}
