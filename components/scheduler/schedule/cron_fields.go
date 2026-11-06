package schedule

// fieldSpec describes one cron field's ordinal range and optional names.
type fieldSpec struct {
	name     string
	min      uint32
	max      uint32
	allowAny bool
	names    map[string]uint32
}

var monthNames = map[string]uint32{
	"jan": 1, "january": 1,
	"feb": 2, "february": 2,
	"mar": 3, "march": 3,
	"apr": 4, "april": 4,
	"may": 5,
	"jun": 6, "june": 6,
	"jul": 7, "july": 7,
	"aug": 8, "august": 8,
	"sep": 9, "september": 9,
	"oct": 10, "october": 10,
	"nov": 11, "november": 11,
	"dec": 12, "december": 12,
}

// Numeric weekdays follow the Rust crate: Sunday is 1 and Saturday is 7.
var weekdayNames = map[string]uint32{
	"sun": 1, "sunday": 1,
	"mon": 2, "monday": 2,
	"tue": 3, "tues": 3, "tuesday": 3,
	"wed": 4, "wednesday": 4,
	"thu": 5, "thurs": 5, "thursday": 5,
	"fri": 6, "friday": 6,
	"sat": 7, "saturday": 7,
}

// Only the day-of-month and day-of-week fields accept '?'.
var (
	minuteField = fieldSpec{name: "Minutes", min: 0, max: 59}
	hourField   = fieldSpec{name: "Hours", min: 0, max: 23}
	domField    = fieldSpec{name: "Days of Month", min: 1, max: 31, allowAny: true}
	monthField  = fieldSpec{name: "Months", min: 1, max: 12, names: monthNames}
	dowField    = fieldSpec{name: "Days of Week", min: 1, max: 7, allowAny: true, names: weekdayNames}
)
