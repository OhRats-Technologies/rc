use chrono::NaiveDateTime;
use rc_scheduler::next_occurrence;

fn millis(value: &str) -> u64 {
    NaiveDateTime::parse_from_str(value, "%Y-%m-%d %H:%M:%S")
        .unwrap()
        .and_utc()
        .timestamp_millis() as u64
}

#[test]
fn normal_occurrence_uses_iana_timezone() {
    let next = next_occurrence(
        "schedule-1",
        "0 9 * * *",
        "America/Toronto",
        millis("2026-01-10 00:00:00"),
        None,
    )
    .unwrap()
    .unwrap();
    assert_eq!(next.scheduled_at_ms, millis("2026-01-10 14:00:00"));
    assert_eq!(next.id, "schedule-1:2026-01-10T09:00");
}

#[test]
fn spring_forward_nonexistent_wall_time_is_skipped() {
    let next = next_occurrence(
        "schedule-1",
        "30 2 * * *",
        "America/Toronto",
        millis("2026-03-08 00:00:00"),
        None,
    )
    .unwrap()
    .unwrap();
    assert_eq!(next.id, "schedule-1:2026-03-09T02:30");
}

#[test]
fn fall_back_wall_time_runs_at_most_once() {
    let first = next_occurrence(
        "schedule-1",
        "30 1 * * *",
        "America/Toronto",
        millis("2026-11-01 00:00:00"),
        None,
    )
    .unwrap()
    .unwrap();
    let second = next_occurrence(
        "schedule-1",
        "30 1 * * *",
        "America/Toronto",
        first.scheduled_at_ms,
        Some(&first.id),
    )
    .unwrap()
    .unwrap();
    assert_eq!(first.id, "schedule-1:2026-11-01T01:30");
    assert_eq!(second.id, "schedule-1:2026-11-02T01:30");
}
