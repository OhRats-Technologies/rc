package diagstore

import (
	"errors"
	"sync"
	"time"

	"wit_component/ohrats_rc_diagnostics_types"
)

const (
	maxEvents = 256
	maxFields = 16
)

var (
	lock     sync.Mutex
	events   []ohrats_rc_diagnostics_types.Event
	sequence uint64
)

// Submit validates and retains one bounded diagnostic report.
func Submit(value ohrats_rc_diagnostics_types.Report) (uint64, error) {
	if err := validateReport(value); err != nil {
		return 0, err
	}
	lock.Lock()
	defer lock.Unlock()
	if sequence < ^uint64(0) {
		sequence++
	}
	event := ohrats_rc_diagnostics_types.Event{
		Sequence:    sequence,
		TimestampMs: timestampMs(),
		Level:       value.Level,
		Source:      value.Source,
		Code:        value.Code,
		Message:     value.Message,
		Fields:      value.Fields,
	}
	events = append(events, event)
	if len(events) > maxEvents {
		excess := len(events) - maxEvents
		copy(events, events[excess:])
		events = events[:maxEvents]
	}
	return sequence, nil
}

// Recent returns up to limit newest retained events.
func Recent(limit uint32) ([]ohrats_rc_diagnostics_types.Event, error) {
	if limit > 100 {
		return nil, errors.New("diagnostic query limit exceeds 100")
	}
	lock.Lock()
	defer lock.Unlock()
	count := int(limit)
	if count > len(events) {
		count = len(events)
	}
	result := make([]ohrats_rc_diagnostics_types.Event, 0, count)
	for index := len(events) - 1; index >= 0 && len(result) < count; index-- {
		result = append(result, cloneEvent(events[index]))
	}
	return result, nil
}

// Status summarizes the retained diagnostic window.
func Status() ohrats_rc_diagnostics_types.Health {
	lock.Lock()
	defer lock.Unlock()
	health := ohrats_rc_diagnostics_types.Health{Retained: uint32(len(events))}
	if len(events) > 0 {
		health.NewestSequence = events[len(events)-1].Sequence
	}
	for _, event := range events {
		switch event.Level {
		case ohrats_rc_diagnostics_types.Level_Error:
			health.Errors++
		case ohrats_rc_diagnostics_types.Level_Warn:
			health.Warnings++
		}
	}
	return health
}

func cloneEvent(value ohrats_rc_diagnostics_types.Event) ohrats_rc_diagnostics_types.Event {
	fields := make([]ohrats_rc_diagnostics_types.Field, len(value.Fields))
	copy(fields, value.Fields)
	value.Fields = fields
	return value
}

func timestampMs() uint64 {
	now := time.Now().UnixNano()
	if now < 0 {
		return 0
	}
	return uint64(now / int64(time.Millisecond))
}
