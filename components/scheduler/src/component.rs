wit_bindgen::generate!({
    path: "../../wit",
    world: "scheduler",
    generate_all,
});

use exports::ohrats::rc_scheduler::driver::{Guest as DriverGuest, TickResult};
use exports::ohrats::rc_scheduler::evaluator::Guest as EvaluatorGuest;
use exports::ohrats::rc_scheduler::management::Guest as ManagementGuest;
use ohrats::{
    rc_diagnostics::{
        reporting,
        types::{Field, Level, Report},
    },
    rc_plugin::types::{Requirement, Selection, Service},
    rc_scheduler::types::{Definition, Occurrence},
};

struct Scheduler;

impl Guest for Scheduler {
    fn descriptor() -> Descriptor {
        Descriptor {
            id: "ohrats:scheduler".into(),
            version: "0.1.0".into(),
            provides: ["evaluator", "management", "driver"]
                .into_iter()
                .map(|name| Service {
                    name: format!("ohrats:rc-scheduler/{name}"),
                    version: "0.1.0".into(),
                    priority: 100,
                    keys: Vec::new(),
                })
                .collect(),
            requires: vec![Requirement {
                name: "ohrats:rc-diagnostics/reporting".into(),
                version: "^0.1".into(),
                selection: Selection::Single,
            }],
            commands: Vec::new(),
        }
    }

    fn activate() -> Result<(), String> {
        reporting::submit(&Report {
            level: Level::Info,
            source: "rc.scheduler".into(),
            code: "scheduler.active".into(),
            message: "portable scheduler activated".into(),
            fields: vec![
                Field {
                    name: "timezone_database".into(),
                    value: "iana".into(),
                },
                Field {
                    name: "overlap_default".into(),
                    value: "forbid".into(),
                },
            ],
        })
        .map(|_| ())
    }

    fn deactivate() {}

    fn invoke(command: String, _args: Vec<String>) -> Result<u32, String> {
        Err(format!("unsupported command {command:?}"))
    }
}

impl DriverGuest for Scheduler {
    fn tick(
        now_unix_ms: u64,
        recovering: bool,
        active_schedule_ids: Vec<String>,
    ) -> Result<TickResult, String> {
        crate::driver::tick(now_unix_ms, recovering, active_schedule_ids)
    }
}

impl ManagementGuest for Scheduler {
    fn list_schedules() -> Result<Vec<Definition>, String> {
        crate::storage::list()
    }

    fn get_schedule(id: String) -> Result<Option<Definition>, String> {
        crate::storage::get(&id)
    }

    fn upsert_schedule(schedule: Definition) -> Result<(), String> {
        validate_definition(&schedule)?;
        crate::storage::put(schedule)
    }

    fn remove_schedule(id: String) -> Result<bool, String> {
        crate::storage::remove(&id)
    }

    fn set_enabled(id: String, enabled: bool) -> Result<bool, String> {
        let Some(mut schedule) = crate::storage::get(&id)? else {
            return Ok(false);
        };
        schedule.enabled = enabled;
        crate::storage::put(schedule)?;
        Ok(true)
    }
}

fn validate_definition(schedule: &Definition) -> Result<(), String> {
    if schedule.permit_hash.is_empty() || schedule.created_by.is_empty() {
        return Err("schedule authority metadata is required".into());
    }
    crate::next_occurrence(
        &schedule.id,
        &schedule.cron,
        &schedule.timezone,
        ohrats::rc_process::clock_host::now_unix_ms(),
        None,
    )?;
    Ok(())
}

impl EvaluatorGuest for Scheduler {
    fn next_occurrence(
        schedule: Definition,
        after_unix_ms: u64,
        last_occurrence_id: Option<String>,
    ) -> Result<Option<Occurrence>, String> {
        if !schedule.enabled
            || schedule
                .expires_at_ms
                .is_some_and(|value| value <= after_unix_ms)
        {
            return Ok(None);
        }
        crate::next_occurrence(
            &schedule.id,
            &schedule.cron,
            &schedule.timezone,
            after_unix_ms,
            last_occurrence_id.as_deref(),
        )
        .map(|value| {
            value.map(|value| Occurrence {
                id: value.id,
                scheduled_at_ms: value.scheduled_at_ms,
            })
        })
    }
}

export!(Scheduler);
