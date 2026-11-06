package cli

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"wit_component/ohrats_rc_diagnostics_types"
)

func levelName(level ohrats_rc_diagnostics_types.Level) string {
	switch level {
	case ohrats_rc_diagnostics_types.Level_Trace:
		return "Trace"
	case ohrats_rc_diagnostics_types.Level_Debug:
		return "Debug"
	case ohrats_rc_diagnostics_types.Level_Info:
		return "Info"
	case ohrats_rc_diagnostics_types.Level_Warn:
		return "Warn"
	case ohrats_rc_diagnostics_types.Level_Error:
		return "Error"
	default:
		return fmt.Sprintf("Level(%d)", level)
	}
}

// escaped replaces control characters so journal and process text cannot inject
// terminal sequences.
func escaped(value string) string {
	var builder strings.Builder
	for _, character := range value {
		if !unicode.IsControl(character) {
			builder.WriteRune(character)
			continue
		}
		switch character {
		case '\t':
			builder.WriteString(`\t`)
		case '\r':
			builder.WriteString(`\r`)
		case '\n':
			builder.WriteString(`\n`)
		default:
			fmt.Fprintf(&builder, `\u{%x}`, character)
		}
	}
	return builder.String()
}

// debugStrings renders a vector of strings with Rust Debug quoting so argument
// boundaries stay visible.
func debugStrings(values []string) string {
	var builder strings.Builder
	builder.WriteByte('[')
	for index, value := range values {
		if index > 0 {
			builder.WriteString(", ")
		}
		debugString(&builder, value)
	}
	builder.WriteByte(']')
	return builder.String()
}

func debugString(builder *strings.Builder, value string) {
	builder.WriteByte('"')
	for _, character := range value {
		switch character {
		case '\\':
			builder.WriteString(`\\`)
		case '"':
			builder.WriteString(`\"`)
		case '\n':
			builder.WriteString(`\n`)
		case '\r':
			builder.WriteString(`\r`)
		case '\t':
			builder.WriteString(`\t`)
		default:
			// Rust Debug escapes non-printable and grapheme-extended characters.
			if unicode.IsPrint(character) && !unicode.Is(unicode.M, character) {
				builder.WriteRune(character)
			} else {
				fmt.Fprintf(builder, `\u{%x}`, character)
			}
		}
	}
	builder.WriteByte('"')
}

// journalTimestamp renders journald microsecond timestamps as UTC calendar
// time without depending on a time zone database.
func journalTimestamp(value string) string {
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return "[unknown time]"
	}
	seconds := parsed / 1_000_000
	days := seconds/86400 + 719468
	era := days / 146097
	day := days % 146097
	yearInEra := (day - day/1460 + day/36524 - day/146096) / 365
	dayInYear := day - (365*yearInEra + yearInEra/4 - yearInEra/100)
	monthFromMarch := (5*dayInYear + 2) / 153
	date := dayInYear - (153*monthFromMarch+2)/5 + 1
	month := monthFromMarch + 3
	if monthFromMarch >= 10 {
		month = monthFromMarch - 9
	}
	year := yearInEra + era*400
	if month <= 2 {
		year++
	}
	return fmt.Sprintf(
		"%04d-%02d-%02d %02d:%02d:%02dZ",
		year,
		month,
		date,
		seconds/3600%24,
		seconds/60%60,
		seconds%60,
	)
}
