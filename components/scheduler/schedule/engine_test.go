package schedule

import "testing"

type memoryStore struct {
	cursors map[string]Cursor
}

func (store *memoryStore) Get(id string) (*Cursor, error) {
	value, ok := store.cursors[id]
	if !ok {
		return nil, nil
	}
	return &value, nil
}

func (store *memoryStore) Put(id string, value Cursor) error {
	if store.cursors == nil {
		store.cursors = make(map[string]Cursor)
	}
	store.cursors[id] = value
	return nil
}

func testSchedule(createdAtMs uint64, skipMisfire bool) Schedule {
	return Schedule{
		ID:          "schedule-1",
		Cron:        "* * * * *",
		Timezone:    "UTC",
		Enabled:     true,
		SkipMisfire: skipMisfire,
		CreatedAtMs: createdAtMs,
	}
}

func TestFakeClockFiresOnceAndPersistsBeforeRepeatTick(t *testing.T) {
	start := utcMillis("2026-01-01 00:00:00")
	now := utcMillis("2026-01-01 00:01:00")
	schedules := []Schedule{testSchedule(start, false)}
	store := &memoryStore{}
	first, err := Tick(store, schedules, now, false, nil)
	if err != nil {
		t.Fatalf("first tick: %v", err)
	}
	second, err := Tick(store, schedules, now, false, nil)
	if err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if len(first.Triggers) != 1 || first.Triggers[0].ScheduleID != "schedule-1" {
		t.Fatalf("first triggers %#v", first.Triggers)
	}
	if len(second.Triggers) != 0 {
		t.Fatalf("second triggers %#v", second.Triggers)
	}
	wake := utcMillis("2026-01-01 00:02:00")
	if first.NextWakeMs == nil || *first.NextWakeMs != wake {
		t.Fatalf("next wake %#v", first.NextWakeMs)
	}
	cursor := store.cursors["schedule-1"]
	if cursor.LastOccurrenceID == nil || *cursor.LastOccurrenceID != "schedule-1:2026-01-01T00:01" {
		t.Fatalf("cursor %#v", cursor)
	}
}

func TestOverlapDisabledAndExpiredSchedulesDoNotFire(t *testing.T) {
	start := utcMillis("2026-01-01 00:00:00")
	now := utcMillis("2026-01-01 00:01:00")
	overlap, err := Tick(&memoryStore{}, []Schedule{testSchedule(start, false)}, now, false, []string{"schedule-1"})
	if err != nil {
		t.Fatalf("overlap tick: %v", err)
	}
	if len(overlap.Triggers) != 0 {
		t.Fatalf("overlap triggers %#v", overlap.Triggers)
	}
	disabled := testSchedule(start, false)
	disabled.Enabled = false
	expired := testSchedule(start, false)
	expired.ID = "expired"
	expires := now
	expired.ExpiresAtMs = &expires
	inactive, err := Tick(&memoryStore{}, []Schedule{disabled, expired}, now, false, nil)
	if err != nil {
		t.Fatalf("inactive tick: %v", err)
	}
	if len(inactive.Triggers) != 0 {
		t.Fatalf("inactive triggers %#v", inactive.Triggers)
	}
}

func TestRecoveryObeysSkipAndRunOnceMisfirePolicies(t *testing.T) {
	start := utcMillis("2026-01-01 00:00:00")
	now := utcMillis("2026-01-01 01:00:00")
	skipped, err := Tick(&memoryStore{}, []Schedule{testSchedule(start, true)}, now, true, nil)
	if err != nil {
		t.Fatalf("skip tick: %v", err)
	}
	if len(skipped.Triggers) != 0 {
		t.Fatalf("skip triggers %#v", skipped.Triggers)
	}
	fired, err := Tick(&memoryStore{}, []Schedule{testSchedule(start, false)}, now, true, nil)
	if err != nil {
		t.Fatalf("run-once tick: %v", err)
	}
	if len(fired.Triggers) != 1 {
		t.Fatalf("run-once triggers %#v", fired.Triggers)
	}
	if fired.Triggers[0].Occurrence.ID != "schedule-1:2026-01-01T01:00" {
		t.Fatalf("occurrence %#v", fired.Triggers[0].Occurrence)
	}
}

func TestBackwardClockDoesNotReplayAnOccurrence(t *testing.T) {
	start := utcMillis("2026-01-01 00:00:00")
	later := utcMillis("2026-01-01 00:05:00")
	earlier := utcMillis("2026-01-01 00:03:00")
	schedules := []Schedule{testSchedule(start, false)}
	store := &memoryStore{}
	first, err := Tick(store, schedules, later, false, nil)
	if err != nil {
		t.Fatalf("first tick: %v", err)
	}
	if len(first.Triggers) != 1 {
		t.Fatalf("first triggers %#v", first.Triggers)
	}
	second, err := Tick(store, schedules, earlier, false, nil)
	if err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if len(second.Triggers) != 0 {
		t.Fatalf("second triggers %#v", second.Triggers)
	}
}
