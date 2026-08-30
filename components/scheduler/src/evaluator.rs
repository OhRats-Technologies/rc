use chrono::{DateTime, TimeZone as _, Utc};
use chrono_tz::Tz;
use cron::Schedule;
use std::str::FromStr as _;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Occurrence {
    pub id: String,
    pub scheduled_at_ms: u64,
}

pub fn next_occurrence(
    schedule_id: &str,
    expression: &str,
    timezone: &str,
    after_unix_ms: u64,
    last_occurrence_id: Option<&str>,
) -> Result<Option<Occurrence>, String> {
    validate_id(schedule_id)?;
    let expression = normalize(expression)?;
    let schedule = Schedule::from_str(&expression).map_err(|error| error.to_string())?;
    let timezone = Tz::from_str(timezone).map_err(|_| "invalid IANA timezone".to_owned())?;
    let after = millis(after_unix_ms)?.with_timezone(&timezone);
    for candidate in schedule.after(&after).take(4_096) {
        let id = occurrence_id(schedule_id, candidate);
        if last_occurrence_id != Some(id.as_str()) {
            let scheduled_at_ms = u64::try_from(candidate.timestamp_millis())
                .map_err(|_| "scheduled instant is before the Unix epoch".to_owned())?;
            return Ok(Some(Occurrence {
                id,
                scheduled_at_ms,
            }));
        }
    }
    Ok(None)
}

fn normalize(expression: &str) -> Result<String, String> {
    let fields: Vec<_> = expression.split_whitespace().collect();
    if fields.len() != 5 {
        return Err("RC schedules require a five-field cron expression".into());
    }
    Ok(format!("0 {} *", fields.join(" ")))
}

fn millis(value: u64) -> Result<DateTime<Utc>, String> {
    let value = i64::try_from(value).map_err(|_| "timestamp exceeds scheduler range".to_owned())?;
    Utc.timestamp_millis_opt(value)
        .single()
        .ok_or_else(|| "invalid scheduler timestamp".to_owned())
}

fn occurrence_id(schedule_id: &str, value: DateTime<Tz>) -> String {
    format!("{schedule_id}:{}", value.format("%Y-%m-%dT%H:%M"))
}

fn validate_id(value: &str) -> Result<(), String> {
    (!value.is_empty()
        && value
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'_' | b'.')))
    .then_some(())
    .ok_or_else(|| "invalid schedule id".to_owned())
}
