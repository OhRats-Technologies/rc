package schedule

import (
	"errors"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	driverTypes "wit_component/ohrats_rc_scheduler_driver"
	types "wit_component/ohrats_rc_scheduler_types"
)

type durableCursors struct{}

func (durableCursors) Get(id string) (*Cursor, error) {
	return cursor(id)
}

func (durableCursors) Put(id string, value Cursor) error {
	return putCursor(id, value)
}

// RunTick loads durable definitions, advances their cursors and returns the
// triggers to execute with their next wake time.
func RunTick(nowMs uint64, recovering bool, activeIDs []string) (driverTypes.TickResult, error) {
	definitions, err := List()
	if err != nil {
		return driverTypes.TickResult{}, err
	}
	schedules := make([]Schedule, 0, len(definitions))
	for _, definition := range definitions {
		schedules = append(schedules, viewSchedule(definition))
	}
	fired, err := Tick(durableCursors{}, schedules, nowMs, recovering, activeIDs)
	if err != nil {
		return driverTypes.TickResult{}, err
	}
	remaining := make(map[string]types.Definition, len(definitions))
	for _, definition := range definitions {
		remaining[definition.Id] = definition
	}
	triggers := make([]driverTypes.Trigger, 0, len(fired.Triggers))
	for _, trigger := range fired.Triggers {
		definition, ok := remaining[trigger.ScheduleID]
		if !ok {
			return driverTypes.TickResult{}, errors.New("scheduler definition disappeared during tick")
		}
		delete(remaining, trigger.ScheduleID)
		triggers = append(triggers, driverTypes.Trigger{
			Schedule: definition,
			Occurrence: types.Occurrence{
				Id:            trigger.Occurrence.ID,
				ScheduledAtMs: trigger.Occurrence.ScheduledAtMs,
			},
		})
	}
	result := driverTypes.TickResult{
		Triggers:       triggers,
		NextWakeUnixMs: witTypes.None[uint64](),
	}
	if fired.NextWakeMs != nil {
		result.NextWakeUnixMs = witTypes.Some(*fired.NextWakeMs)
	}
	return result, nil
}

func viewSchedule(value types.Definition) Schedule {
	return Schedule{
		ID:          value.Id,
		Cron:        value.Cron,
		Timezone:    value.Timezone,
		Enabled:     value.Enabled,
		SkipMisfire: value.Misfire == types.MisfirePolicy_Skip,
		CreatedAtMs: value.CreatedAtMs,
		ExpiresAtMs: uint64Option(value.ExpiresAtMs),
	}
}
