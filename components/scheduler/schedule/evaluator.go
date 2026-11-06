package schedule

import (
	"fmt"
	"time"
	_ "time/tzdata"
)

// Occurrence is a single scheduled firing. The identifier is deterministic:
// the schedule id plus the local wall-clock minute in the schedule timezone.
type Occurrence struct {
	ID            string
	ScheduledAtMs uint64
}

const (
	maxOccurrenceCandidates = 4096
	occurrenceIDLayout      = "2006-01-02T15:04"
)

// NextOccurrence returns the first occurrence strictly after afterUnixMs,
// skipping the occurrence named by lastOccurrenceID so a fall-back wall time is
// never emitted twice.
func NextOccurrence(scheduleID, expression, timezone string, afterUnixMs uint64, lastOccurrenceID *string) (*Occurrence, error) {
	if err := validateScheduleID(scheduleID); err != nil {
		return nil, err
	}
	parsed, err := ParseExpression(expression)
	if err != nil {
		return nil, err
	}
	location, err := loadTimezone(timezone)
	if err != nil {
		return nil, err
	}
	after, err := millis(afterUnixMs)
	if err != nil {
		return nil, err
	}
	lower := wallLowerBound(after, location)
	skipped := 0
	for skipped < maxOccurrenceCandidates {
		wall, ok := parsed.nextWall(lower)
		if !ok {
			return nil, nil
		}
		instant, exists := resolveWall(wall, location)
		if !exists {
			// A spring-forward wall time does not exist; the cron crate skips it
			// without consuming iterator budget.
			lower = wall.Add(time.Minute)
			continue
		}
		skipped++
		id := occurrenceID(scheduleID, instant.In(location))
		if lastOccurrenceID != nil && id == *lastOccurrenceID {
			lower = wall.Add(time.Minute)
			continue
		}
		milliseconds := instant.UnixMilli()
		if milliseconds < 0 {
			return nil, fmt.Errorf("scheduled instant is before the Unix epoch")
		}
		return &Occurrence{ID: id, ScheduledAtMs: uint64(milliseconds)}, nil
	}
	return nil, nil
}

func validateScheduleID(value string) error {
	if value == "" {
		return fmt.Errorf("invalid schedule id")
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		isAlphaNumeric := (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9')
		if !isAlphaNumeric && character != '-' && character != '_' && character != '.' {
			return fmt.Errorf("invalid schedule id")
		}
	}
	return nil
}

func loadTimezone(name string) (*time.Location, error) {
	if name == "" || name == "Local" {
		return nil, fmt.Errorf("invalid IANA timezone")
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("invalid IANA timezone")
	}
	return location, nil
}

func millis(value uint64) (time.Time, error) {
	if value > uint64(1<<63-1) {
		return time.Time{}, fmt.Errorf("timestamp exceeds scheduler range")
	}
	result := time.UnixMilli(int64(value)).UTC()
	// chrono cannot represent instants outside years -262144..=262143.
	if result.Year() > 262143 {
		return time.Time{}, fmt.Errorf("invalid scheduler timestamp")
	}
	return result, nil
}

func occurrenceID(scheduleID string, local time.Time) string {
	return scheduleID + ":" + local.Format(occurrenceIDLayout)
}

// wallLowerBound converts the cron crate's inclusive lower bound into local
// wall-clock components. The cron iterator starts one second after the cursor
// and only ever emits whole-minute wall times, so a cursor late in a minute
// moves the search to the following minute.
func wallLowerBound(after time.Time, location *time.Location) time.Time {
	local := after.In(location).Add(time.Second)
	year, month, day := local.Date()
	hour, minute, second := local.Clock()
	wall := time.Date(year, month, day, hour, minute, 0, 0, time.UTC)
	if second != 0 {
		wall = wall.Add(time.Minute)
	}
	return wall
}

// nextWall finds the first matching wall-clock minute at or after lower. Wall
// times are represented in UTC so DST transitions never affect iteration.
func (expression *Expression) nextWall(lower time.Time) (time.Time, bool) {
	start := lower
	if start.Year() < cronMinYear {
		start = time.Date(cronMinYear, time.January, 1, 0, 0, 0, 0, time.UTC)
	}
	if start.Year() > cronMaxYear {
		return time.Time{}, false
	}
	first := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	for day := first; day.Year() <= cronMaxYear; day = day.AddDate(0, 0, 1) {
		if !expression.matchesMonth(uint32(day.Month())) {
			continue
		}
		weekday := uint32(day.Weekday()) + 1
		if !expression.matchesDay(uint32(day.Day()), weekday) {
			continue
		}
		hourStart := 0
		minuteStart := 0
		if day.Equal(first) {
			hourStart = start.Hour()
			minuteStart = start.Minute()
		}
		for hour := hourStart; hour < 24; hour++ {
			if expression.hours&(uint64(1)<<uint32(hour)) == 0 {
				continue
			}
			minuteBegin := 0
			if day.Equal(first) && hour == hourStart {
				minuteBegin = minuteStart
			}
			for minute := minuteBegin; minute < 60; minute++ {
				if expression.minutes&(uint64(1)<<uint32(minute)) == 0 {
					continue
				}
				return time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, time.UTC), true
			}
		}
	}
	return time.Time{}, false
}

// resolveWall maps wall-clock components to the earliest instant that displays
// them in location. It reports false for wall times skipped by a forward
// transition, and returns the earlier instant for repeated wall times.
func resolveWall(wall time.Time, location *time.Location) (time.Time, bool) {
	year, month, day := wall.Date()
	hour, minute, _ := wall.Clock()
	want := time.Date(year, month, day, hour, minute, 0, 0, time.UTC)
	seen := make(map[int64]struct{})
	var earliest time.Time
	found := false
	for _, probe := range []time.Duration{
		-48 * time.Hour, -24 * time.Hour, 0, 24 * time.Hour, 48 * time.Hour,
	} {
		_, offset := want.Add(probe).In(location).Zone()
		instant := want.Add(-time.Duration(offset) * time.Second)
		if _, ok := seen[instant.Unix()]; ok {
			continue
		}
		seen[instant.Unix()] = struct{}{}
		local := instant.In(location)
		localYear, localMonth, localDay := local.Date()
		localHour, localMinute, _ := local.Clock()
		if localYear != year || localMonth != month || localDay != day ||
			localHour != hour || localMinute != minute {
			continue
		}
		if !found || instant.Before(earliest) {
			earliest = instant
			found = true
		}
	}
	return earliest, found
}
