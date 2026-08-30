use std::collections::HashSet;

const MAX_ADVANCE: usize = 1_024;

#[derive(Clone)]
pub struct Schedule {
    pub id: String,
    pub cron: String,
    pub timezone: String,
    pub enabled: bool,
    pub skip_misfire: bool,
    pub created_at_ms: u64,
    pub expires_at_ms: Option<u64>,
}

#[derive(Clone)]
pub struct Cursor {
    pub checked_at_ms: u64,
    pub last_occurrence_id: Option<String>,
}

pub struct Trigger {
    pub schedule_id: String,
    pub occurrence: crate::Occurrence,
}

pub struct TickResult {
    pub triggers: Vec<Trigger>,
    pub next_wake_ms: Option<u64>,
}

pub trait CursorStore {
    fn get(&mut self, id: &str) -> Result<Option<Cursor>, String>;
    fn put(&mut self, id: &str, value: &Cursor) -> Result<(), String>;
}

pub fn tick(
    store: &mut impl CursorStore,
    schedules: Vec<Schedule>,
    now_ms: u64,
    recovering: bool,
    active_ids: Vec<String>,
) -> Result<TickResult, String> {
    let active: HashSet<_> = active_ids.into_iter().collect();
    let mut triggers = Vec::new();
    let mut next_wake_ms = None;
    for schedule in schedules {
        let (trigger, wake) = tick_schedule(store, schedule, now_ms, recovering, &active)?;
        if let Some(trigger) = trigger {
            triggers.push(trigger);
        }
        next_wake_ms = [next_wake_ms, wake].into_iter().flatten().min();
    }
    Ok(TickResult {
        triggers,
        next_wake_ms,
    })
}

fn tick_schedule(
    store: &mut impl CursorStore,
    schedule: Schedule,
    now_ms: u64,
    recovering: bool,
    active: &HashSet<String>,
) -> Result<(Option<Trigger>, Option<u64>), String> {
    if !schedule.enabled || schedule.expires_at_ms.is_some_and(|value| value <= now_ms) {
        return Ok((None, None));
    }
    let prior = store.get(&schedule.id)?.unwrap_or(Cursor {
        checked_at_ms: schedule.created_at_ms,
        last_occurrence_id: None,
    });
    if now_ms < prior.checked_at_ms {
        return Ok((None, Some(prior.checked_at_ms)));
    }
    let mut after = prior.checked_at_ms;
    let mut latest = None;
    for _ in 0..MAX_ADVANCE {
        let Some(next) = crate::next_occurrence(
            &schedule.id,
            &schedule.cron,
            &schedule.timezone,
            after,
            prior.last_occurrence_id.as_deref(),
        )?
        else {
            break;
        };
        if next.scheduled_at_ms > now_ms {
            break;
        }
        after = next.scheduled_at_ms;
        latest = Some(next);
    }
    let last_occurrence_id = latest
        .as_ref()
        .map(|value| value.id.clone())
        .or(prior.last_occurrence_id);
    store.put(
        &schedule.id,
        &Cursor {
            checked_at_ms: now_ms,
            last_occurrence_id: last_occurrence_id.clone(),
        },
    )?;
    let next_wake = crate::next_occurrence(
        &schedule.id,
        &schedule.cron,
        &schedule.timezone,
        now_ms,
        last_occurrence_id.as_deref(),
    )?
    .map(|value| value.scheduled_at_ms);
    let trigger = latest.and_then(|occurrence| {
        (!active.contains(&schedule.id) && !(recovering && schedule.skip_misfire)).then_some({
            Trigger {
                schedule_id: schedule.id,
                occurrence,
            }
        })
    });
    Ok((trigger, next_wake))
}

#[cfg(test)]
mod tests;
