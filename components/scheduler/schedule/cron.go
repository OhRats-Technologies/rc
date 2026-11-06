package schedule

import (
	"fmt"
	"strconv"
	"strings"
)

// Expression is a parsed five-field cron expression. It mirrors the subset of
// the Rust `cron` crate used by RC: wildcard, point, range, named range and
// period specifiers with comma-separated lists. Fields are held as bitmasks of
// their ordinal values.
type Expression struct {
	minutes       uint64
	hours         uint64
	daysOfMonth   uint64
	months        uint64
	daysOfWeek    uint64
	daysOfWeekAll bool
}

const (
	cronMinYear = 1970
	cronMaxYear = 2100
)

// ParseExpression parses an RC cron expression. RC requires five fields
// (minute, hour, day of month, month, day of week); the original Rust
// implementation prepends a zero seconds field and appends an unrestricted
// year field before handing the expression to the cron crate.
func ParseExpression(expression string) (*Expression, error) {
	fields := strings.Fields(expression)
	if len(fields) != 5 {
		return nil, fmt.Errorf("RC schedules require a five-field cron expression")
	}
	minutes, err := parseField(fields[0], minuteField)
	if err != nil {
		return nil, err
	}
	hours, err := parseField(fields[1], hourField)
	if err != nil {
		return nil, err
	}
	daysOfMonth, err := parseField(fields[2], domField)
	if err != nil {
		return nil, err
	}
	months, err := parseField(fields[3], monthField)
	if err != nil {
		return nil, err
	}
	daysOfWeek, err := parseField(fields[4], dowField)
	if err != nil {
		return nil, err
	}
	return &Expression{
		minutes:       minutes,
		hours:         hours,
		daysOfMonth:   daysOfMonth,
		months:        months,
		daysOfWeek:    daysOfWeek,
		daysOfWeekAll: daysOfWeek == allBits(dowField.min, dowField.max),
	}, nil
}

func parseField(field string, spec fieldSpec) (uint64, error) {
	var set uint64
	for _, item := range strings.Split(field, ",") {
		if err := parseItem(item, spec, &set); err != nil {
			return 0, err
		}
	}
	for value := uint32(0); value < 64; value++ {
		if set&(uint64(1)<<value) != 0 && (value < spec.min || value > spec.max) {
			return 0, fmt.Errorf("%s must be between %d and %d. ('%d' specified.)",
				spec.name, spec.min, spec.max, value)
		}
	}
	return set, nil
}

func parseItem(item string, spec fieldSpec, set *uint64) error {
	if item == "" {
		return fmt.Errorf("empty %s field", spec.name)
	}
	base := item
	var step uint32
	if index := strings.IndexByte(item, '/'); index >= 0 {
		base = item[:index]
		text := item[index+1:]
		value, err := decimal(text)
		if err != nil {
			return fmt.Errorf("invalid period for %s: %q", spec.name, item)
		}
		if value == 0 {
			return fmt.Errorf("range step cannot be zero")
		}
		if value > spec.max {
			return fmt.Errorf("%s must be between 1 and %d. ('%d' specified.)",
				spec.name, spec.max, value)
		}
		step = value
	}
	ordinals, err := parseSpecifier(base, spec, step > 0)
	if err != nil {
		return err
	}
	if step > 0 {
		ordinals = applyStep(ordinals, spec, step)
	}
	*set |= ordinals
	return nil
}

// parseSpecifier returns the ordinal set named by one specifier before any
// period step is applied. A bare name is only accepted without a step.
func parseSpecifier(text string, spec fieldSpec, stepped bool) (uint64, error) {
	switch text {
	case "":
		return 0, fmt.Errorf("empty %s field", spec.name)
	case "*":
		return allBits(spec.min, spec.max), nil
	case "?":
		if !spec.allowAny {
			return 0, fmt.Errorf("the %s field does not support '?'", spec.name)
		}
		return allBits(spec.min, spec.max), nil
	}
	if index := strings.IndexByte(text, '-'); index >= 0 {
		startText, endText := text[:index], text[index+1:]
		if strings.IndexByte(endText, '-') >= 0 {
			return 0, fmt.Errorf("invalid range for %s: %q", spec.name, text)
		}
		if isAlpha(startText) != isAlpha(endText) {
			return 0, fmt.Errorf("invalid range for %s: %q", spec.name, text)
		}
		start, err := ordinal(startText, spec)
		if err != nil {
			return 0, err
		}
		end, err := ordinal(endText, spec)
		if err != nil {
			return 0, err
		}
		if start > end {
			return 0, fmt.Errorf("invalid range for %s: %d-%d", spec.name, start, end)
		}
		return allBits(start, end), nil
	}
	if isAlpha(text) {
		if stepped {
			return 0, fmt.Errorf("invalid period for %s: %q", spec.name, text)
		}
		value, ok := spec.names[strings.ToLower(text)]
		if !ok {
			return 0, fmt.Errorf("%q is not a valid name for %s", text, spec.name)
		}
		return uint64(1) << value, nil
	}
	value, err := decimal(text)
	if err != nil {
		return 0, fmt.Errorf("invalid %s field: %q", spec.name, text)
	}
	if value < spec.min || value > spec.max {
		return 0, fmt.Errorf("%s must be between %d and %d. ('%d' specified.)",
			spec.name, spec.min, spec.max, value)
	}
	if stepped {
		// A point before a period implies a range from that point through the
		// field maximum.
		return allBits(value, spec.max), nil
	}
	return uint64(1) << value, nil
}

func ordinal(text string, spec fieldSpec) (uint32, error) {
	if isAlpha(text) {
		if spec.names == nil {
			return 0, fmt.Errorf("the %s field does not support names", spec.name)
		}
		value, ok := spec.names[strings.ToLower(text)]
		if !ok {
			return 0, fmt.Errorf("%q is not a valid name for %s", text, spec.name)
		}
		return value, nil
	}
	value, err := decimal(text)
	if err != nil {
		return 0, fmt.Errorf("invalid range for %s: %q", spec.name, text)
	}
	if value < spec.min || value > spec.max {
		return 0, fmt.Errorf("%s must be between %d and %d. ('%d' specified.)",
			spec.name, spec.min, spec.max, value)
	}
	return value, nil
}

// applyStep keeps every step-th ordinal of the base set, matching the cron
// crate's range step semantics for point, wildcard and range bases.
func applyStep(base uint64, spec fieldSpec, step uint32) uint64 {
	var result uint64
	var index uint32
	for value := spec.min; value <= spec.max; value++ {
		if base&(uint64(1)<<value) == 0 {
			continue
		}
		if index%step == 0 {
			result |= uint64(1) << value
		}
		index++
	}
	return result
}

func allBits(min, max uint32) uint64 {
	var result uint64
	for value := min; value <= max; value++ {
		result |= uint64(1) << value
	}
	return result
}

func decimal(text string) (uint32, error) {
	if text == "" {
		return 0, fmt.Errorf("empty ordinal")
	}
	value, err := strconv.ParseUint(text, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint32(value), nil
}

func isAlpha(text string) bool {
	if text == "" {
		return false
	}
	for index := 0; index < len(text); index++ {
		letter := text[index]
		if (letter < 'a' || letter > 'z') && (letter < 'A' || letter > 'Z') {
			return false
		}
	}
	return true
}

func (expression *Expression) matchesMonth(month uint32) bool {
	return expression.months&(uint64(1)<<month) != 0
}

// matchesDay applies the cron crate's day filter: the day of month must match,
// and when the day-of-week field is restricted it must match as well.
func (expression *Expression) matchesDay(day uint32, weekday uint32) bool {
	if expression.daysOfMonth&(uint64(1)<<day) == 0 {
		return false
	}
	return expression.daysOfWeekAll || expression.daysOfWeek&(uint64(1)<<weekday) != 0
}
