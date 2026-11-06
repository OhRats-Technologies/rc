package schedule

const maxAdvance = 1024

// Schedule is the minimal definition view the tick engine needs.
type Schedule struct {
	ID          string
	Cron        string
	Timezone    string
	Enabled     bool
	SkipMisfire bool
	CreatedAtMs uint64
	ExpiresAtMs *uint64
}

// Cursor is the persisted per-schedule tick position.
type Cursor struct {
	CheckedAtMs      uint64
	LastOccurrenceID *string
}

// Trigger identifies a schedule occurrence emitted by a tick.
type Trigger struct {
	ScheduleID string
	Occurrence Occurrence
}

// Fired is the result of one engine tick.
type Fired struct {
	Triggers   []Trigger
	NextWakeMs *uint64
}

// CursorStore persists cursor state. The production implementation is backed by
// the durable store; tests can use an in-memory store.
type CursorStore interface {
	Get(id string) (*Cursor, error)
	Put(id string, value Cursor) error
}

// Tick advances every schedule to nowMs, persisting cursors before returning.
func Tick(store CursorStore, schedules []Schedule, nowMs uint64, recovering bool, activeIDs []string) (Fired, error) {
	active := make(map[string]struct{}, len(activeIDs))
	for _, id := range activeIDs {
		active[id] = struct{}{}
	}
	fired := Fired{Triggers: make([]Trigger, 0)}
	for _, schedule := range schedules {
		trigger, wake, err := tickSchedule(store, schedule, nowMs, recovering, active)
		if err != nil {
			return Fired{}, err
		}
		if trigger != nil {
			fired.Triggers = append(fired.Triggers, *trigger)
		}
		if wake != nil && (fired.NextWakeMs == nil || *wake < *fired.NextWakeMs) {
			fired.NextWakeMs = wake
		}
	}
	return fired, nil
}

func tickSchedule(store CursorStore, schedule Schedule, nowMs uint64, recovering bool, active map[string]struct{}) (*Trigger, *uint64, error) {
	if !schedule.Enabled || (schedule.ExpiresAtMs != nil && *schedule.ExpiresAtMs <= nowMs) {
		return nil, nil, nil
	}
	prior, err := store.Get(schedule.ID)
	if err != nil {
		return nil, nil, err
	}
	if prior == nil {
		prior = &Cursor{CheckedAtMs: schedule.CreatedAtMs}
	}
	if nowMs < prior.CheckedAtMs {
		checked := prior.CheckedAtMs
		return nil, &checked, nil
	}
	after := prior.CheckedAtMs
	var latest *Occurrence
	for advance := 0; advance < maxAdvance; advance++ {
		next, err := NextOccurrence(schedule.ID, schedule.Cron, schedule.Timezone, after, prior.LastOccurrenceID)
		if err != nil {
			return nil, nil, err
		}
		if next == nil || next.ScheduledAtMs > nowMs {
			break
		}
		after = next.ScheduledAtMs
		latest = next
	}
	lastOccurrenceID := prior.LastOccurrenceID
	if latest != nil {
		identifier := latest.ID
		lastOccurrenceID = &identifier
	}
	if err := store.Put(schedule.ID, Cursor{
		CheckedAtMs:      nowMs,
		LastOccurrenceID: lastOccurrenceID,
	}); err != nil {
		return nil, nil, err
	}
	next, err := NextOccurrence(schedule.ID, schedule.Cron, schedule.Timezone, nowMs, lastOccurrenceID)
	if err != nil {
		return nil, nil, err
	}
	var wake *uint64
	if next != nil {
		scheduled := next.ScheduledAtMs
		wake = &scheduled
	}
	var trigger *Trigger
	if latest != nil {
		_, isActive := active[schedule.ID]
		if !isActive && !(recovering && schedule.SkipMisfire) {
			trigger = &Trigger{ScheduleID: schedule.ID, Occurrence: *latest}
		}
	}
	return trigger, wake, nil
}
