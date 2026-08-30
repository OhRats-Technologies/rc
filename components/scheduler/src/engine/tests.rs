use super::*;
use chrono::NaiveDateTime;
use std::collections::HashMap;

#[derive(Default)]
struct MemoryStore(HashMap<String, Cursor>);

impl CursorStore for MemoryStore {
    fn get(&mut self, id: &str) -> Result<Option<Cursor>, String> {
        Ok(self.0.get(id).cloned())
    }

    fn put(&mut self, id: &str, value: &Cursor) -> Result<(), String> {
        self.0.insert(id.into(), value.clone());
        Ok(())
    }
}

fn millis(value: &str) -> u64 {
    NaiveDateTime::parse_from_str(value, "%Y-%m-%d %H:%M:%S")
        .unwrap()
        .and_utc()
        .timestamp_millis() as u64
}

fn schedule(created_at_ms: u64, skip_misfire: bool) -> Schedule {
    Schedule {
        id: "schedule-1".into(),
        cron: "* * * * *".into(),
        timezone: "UTC".into(),
        enabled: true,
        skip_misfire,
        created_at_ms,
        expires_at_ms: None,
    }
}

#[test]
fn fake_clock_fires_once_and_persists_before_repeat_tick() {
    let start = millis("2026-01-01 00:00:00");
    let now = millis("2026-01-01 00:01:00");
    let schedules = vec![schedule(start, false)];
    let mut store = MemoryStore::default();
    let first = tick(&mut store, schedules.clone(), now, false, Vec::new()).unwrap();
    let second = tick(&mut store, schedules, now, false, Vec::new()).unwrap();
    assert_eq!(first.triggers.len(), 1);
    assert_eq!(first.triggers[0].schedule_id, "schedule-1");
    assert!(second.triggers.is_empty());
    assert_eq!(first.next_wake_ms, Some(millis("2026-01-01 00:02:00")));
    assert_eq!(
        store.0["schedule-1"].last_occurrence_id.as_deref(),
        Some("schedule-1:2026-01-01T00:01")
    );
}

#[test]
fn overlap_disabled_and_expired_schedules_do_not_fire() {
    let start = millis("2026-01-01 00:00:00");
    let now = millis("2026-01-01 00:01:00");
    let mut store = MemoryStore::default();
    assert!(
        tick(
            &mut store,
            vec![schedule(start, false)],
            now,
            false,
            vec!["schedule-1".into()],
        )
        .unwrap()
        .triggers
        .is_empty()
    );
    let mut disabled = schedule(start, false);
    disabled.enabled = false;
    let mut expired = schedule(start, false);
    expired.id = "expired".into();
    expired.expires_at_ms = Some(now);
    assert!(
        tick(
            &mut MemoryStore::default(),
            vec![disabled, expired],
            now,
            false,
            Vec::new(),
        )
        .unwrap()
        .triggers
        .is_empty()
    );
}

#[test]
fn recovery_obeys_skip_and_run_once_misfire_policies() {
    let start = millis("2026-01-01 00:00:00");
    let now = millis("2026-01-01 01:00:00");
    assert!(
        tick(
            &mut MemoryStore::default(),
            vec![schedule(start, true)],
            now,
            true,
            Vec::new(),
        )
        .unwrap()
        .triggers
        .is_empty()
    );
    let triggers = tick(
        &mut MemoryStore::default(),
        vec![schedule(start, false)],
        now,
        true,
        Vec::new(),
    )
    .unwrap();
    assert_eq!(triggers.triggers.len(), 1);
    assert_eq!(
        triggers.triggers[0].occurrence.id,
        "schedule-1:2026-01-01T01:00"
    );
}

#[test]
fn backward_clock_does_not_replay_an_occurrence() {
    let start = millis("2026-01-01 00:00:00");
    let later = millis("2026-01-01 00:05:00");
    let earlier = millis("2026-01-01 00:03:00");
    let schedules = vec![schedule(start, false)];
    let mut store = MemoryStore::default();
    assert_eq!(
        tick(&mut store, schedules.clone(), later, false, Vec::new())
            .unwrap()
            .triggers
            .len(),
        1
    );
    assert!(
        tick(&mut store, schedules, earlier, false, Vec::new())
            .unwrap()
            .triggers
            .is_empty()
    );
}
