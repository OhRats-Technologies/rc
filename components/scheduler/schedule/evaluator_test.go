package schedule

import (
	"testing"
	"time"
)

func utcMillis(value string) uint64 {
	parsed, err := time.Parse("2006-01-02 15:04:05", value)
	if err != nil {
		panic(err)
	}
	return uint64(parsed.UnixMilli())
}

func TestNormalOccurrenceUsesIANATimezone(t *testing.T) {
	next, err := NextOccurrence("schedule-1", "0 9 * * *", "America/Toronto", utcMillis("2026-01-10 00:00:00"), nil)
	if err != nil {
		t.Fatalf("next occurrence: %v", err)
	}
	if next == nil {
		t.Fatal("expected an occurrence")
	}
	if next.ScheduledAtMs != utcMillis("2026-01-10 14:00:00") {
		t.Fatalf("scheduled at %d", next.ScheduledAtMs)
	}
	if next.ID != "schedule-1:2026-01-10T09:00" {
		t.Fatalf("identifier %q", next.ID)
	}
}

func TestSpringForwardNonexistentWallTimeIsSkipped(t *testing.T) {
	next, err := NextOccurrence("schedule-1", "30 2 * * *", "America/Toronto", utcMillis("2026-03-08 00:00:00"), nil)
	if err != nil {
		t.Fatalf("next occurrence: %v", err)
	}
	if next == nil || next.ID != "schedule-1:2026-03-09T02:30" {
		t.Fatalf("unexpected occurrence %#v", next)
	}
}

func TestFallBackWallTimeRunsAtMostOnce(t *testing.T) {
	first, err := NextOccurrence("schedule-1", "30 1 * * *", "America/Toronto", utcMillis("2026-11-01 00:00:00"), nil)
	if err != nil || first == nil {
		t.Fatalf("first occurrence: %v %#v", err, first)
	}
	if first.ID != "schedule-1:2026-11-01T01:30" {
		t.Fatalf("first identifier %q", first.ID)
	}
	second, err := NextOccurrence("schedule-1", "30 1 * * *", "America/Toronto", first.ScheduledAtMs, &first.ID)
	if err != nil || second == nil {
		t.Fatalf("second occurrence: %v %#v", err, second)
	}
	if second.ID != "schedule-1:2026-11-02T01:30" {
		t.Fatalf("second identifier %q", second.ID)
	}
}

func TestFiveFieldCronIsRequired(t *testing.T) {
	if _, err := NextOccurrence("schedule-1", "0 9 * * * *", "UTC", 0, nil); err == nil {
		t.Fatal("expected six-field expression to fail")
	}
	if _, err := NextOccurrence("schedule-1", "0 9 * *", "UTC", 0, nil); err == nil {
		t.Fatal("expected four-field expression to fail")
	}
	if _, err := NextOccurrence("bad/id", "0 9 * * *", "UTC", 0, nil); err == nil {
		t.Fatal("expected invalid schedule id to fail")
	}
	if _, err := NextOccurrence("schedule-1", "0 9 * * *", "Mars/Olympus", 0, nil); err == nil {
		t.Fatal("expected invalid timezone to fail")
	}
}

func TestCronDayOfMonthAndDayOfWeekBothMatch(t *testing.T) {
	// The cron crate requires both a restricted day of month and day of week.
	next, err := NextOccurrence("schedule-1", "0 0 1 * MON", "UTC", utcMillis("2026-01-02 00:00:00"), nil)
	if err != nil || next == nil {
		t.Fatalf("next occurrence: %v %#v", err, next)
	}
	instant := time.UnixMilli(int64(next.ScheduledAtMs)).UTC()
	if instant.Day() != 1 || instant.Weekday() != time.Monday {
		t.Fatalf("unexpected day %s", instant)
	}
}

func TestCronPointPeriodStartsAtPoint(t *testing.T) {
	after := utcMillis("2026-01-01 00:00:00")
	first, err := NextOccurrence("schedule-1", "5/10 * * * *", "UTC", after, nil)
	if err != nil || first == nil {
		t.Fatalf("first occurrence: %v %#v", err, first)
	}
	if first.ID != "schedule-1:2026-01-01T00:05" {
		t.Fatalf("first identifier %q", first.ID)
	}
	second, err := NextOccurrence("schedule-1", "5/10 * * * *", "UTC", first.ScheduledAtMs, &first.ID)
	if err != nil || second == nil {
		t.Fatalf("second occurrence: %v %#v", err, second)
	}
	if second.ID != "schedule-1:2026-01-01T00:15" {
		t.Fatalf("second identifier %q", second.ID)
	}
}

func TestCronFieldForms(t *testing.T) {
	valid := []string{
		"* * * * *",
		"*/15 * * * *",
		"0 9 * * MON-FRI",
		"0 0 1,15 JAN-JUN *",
		"5/10 * * * *",
		"0 0 ? * ?",
		"0 0 1 1 ?",
		"0 0 1 Jan-March Thurs",
	}
	for _, expression := range valid {
		if _, err := ParseExpression(expression); err != nil {
			t.Fatalf("%q: %v", expression, err)
		}
	}
	invalid := []string{
		"? * * * *",
		"* * * ? *",
		"60 * * * *",
		"* 24 * * *",
		"0 0 0 * *",
		"0 0 * * 0",
		"0 0 31-1 * *",
		"0 0 1-Feb * *",
		"0 0 Jan-2 * *",
		"*/60 * * * *",
		"0 0 * * MON/2",
	}
	for _, expression := range invalid {
		if _, err := ParseExpression(expression); err == nil {
			t.Fatalf("%q: expected parse failure", expression)
		}
	}
}
