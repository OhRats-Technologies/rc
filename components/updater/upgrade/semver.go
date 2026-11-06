// Package upgrade implements the semantic-version rules used by the updater
// when it validates a staged native kernel image.
package upgrade

import (
	"errors"
	"strconv"
	"strings"
)

// Version is a parsed semantic version. Build metadata is retained for
// formatting but never participates in precedence comparison.
type Version struct {
	Major uint64
	Minor uint64
	Patch uint64
	Pre   []Identifier
	Build []string
}

// Identifier is one dot-separated pre-release identifier.
type Identifier struct {
	Numeric bool
	Number  uint64
	Text    string
}

// Parse accepts only complete MAJOR.MINOR.PATCH versions with optional
// pre-release and build metadata, matching strict semver grammar.
func Parse(value string) (Version, error) {
	var parsed Version
	rest := value
	if index := strings.IndexByte(rest, '+'); index >= 0 {
		build, err := parseBuild(rest[index+1:])
		if err != nil {
			return Version{}, err
		}
		parsed.Build = build
		rest = rest[:index]
	}
	if index := strings.IndexByte(rest, '-'); index >= 0 {
		pre, err := parsePre(rest[index+1:])
		if err != nil {
			return Version{}, err
		}
		parsed.Pre = pre
		rest = rest[:index]
	}
	numbers := strings.Split(rest, ".")
	if len(numbers) != 3 {
		return Version{}, errors.New("invalid semantic version")
	}
	fields := []*uint64{&parsed.Major, &parsed.Minor, &parsed.Patch}
	for index, text := range numbers {
		number, err := parseNumeric(text)
		if err != nil {
			return Version{}, err
		}
		*fields[index] = number
	}
	return parsed, nil
}

// String renders the canonical representation of the version.
func (version Version) String() string {
	var builder strings.Builder
	builder.WriteString(strconv.FormatUint(version.Major, 10))
	builder.WriteByte('.')
	builder.WriteString(strconv.FormatUint(version.Minor, 10))
	builder.WriteByte('.')
	builder.WriteString(strconv.FormatUint(version.Patch, 10))
	if len(version.Pre) > 0 {
		builder.WriteByte('-')
		for index, identifier := range version.Pre {
			if index > 0 {
				builder.WriteByte('.')
			}
			builder.WriteString(identifier.String())
		}
	}
	if len(version.Build) > 0 {
		builder.WriteByte('+')
		builder.WriteString(strings.Join(version.Build, "."))
	}
	return builder.String()
}

// String renders the identifier in its canonical form.
func (identifier Identifier) String() string {
	if identifier.Numeric {
		return strconv.FormatUint(identifier.Number, 10)
	}
	return identifier.Text
}

// Compare orders two versions by semver precedence. It returns a negative
// number when left precedes right, zero when they have equal precedence, and
// a positive number otherwise.
func Compare(left Version, right Version) int {
	if result := compareUint(left.Major, right.Major); result != 0 {
		return result
	}
	if result := compareUint(left.Minor, right.Minor); result != 0 {
		return result
	}
	if result := compareUint(left.Patch, right.Patch); result != 0 {
		return result
	}
	return comparePre(left.Pre, right.Pre)
}

func comparePre(left []Identifier, right []Identifier) int {
	switch {
	case len(left) == 0 && len(right) == 0:
		return 0
	case len(left) == 0:
		return 1
	case len(right) == 0:
		return -1
	}
	for index := 0; index < len(left) && index < len(right); index++ {
		if result := compareIdentifier(left[index], right[index]); result != 0 {
			return result
		}
	}
	return compareUint(uint64(len(left)), uint64(len(right)))
}

func compareIdentifier(left Identifier, right Identifier) int {
	if left.Numeric && right.Numeric {
		return compareUint(left.Number, right.Number)
	}
	if left.Numeric != right.Numeric {
		if left.Numeric {
			return -1
		}
		return 1
	}
	return strings.Compare(left.Text, right.Text)
}

func compareUint(left uint64, right uint64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func parsePre(value string) ([]Identifier, error) {
	if value == "" {
		return nil, errors.New("empty pre-release")
	}
	parts := strings.Split(value, ".")
	identifiers := make([]Identifier, 0, len(parts))
	for _, part := range parts {
		if part == "" || !alphanumericHyphen(part) {
			return nil, errors.New("invalid pre-release")
		}
		if numeric(part) {
			number, err := parseNumeric(part)
			if err != nil {
				return nil, err
			}
			identifiers = append(identifiers, Identifier{Numeric: true, Number: number})
			continue
		}
		identifiers = append(identifiers, Identifier{Text: part})
	}
	return identifiers, nil
}

func parseBuild(value string) ([]string, error) {
	if value == "" {
		return nil, errors.New("empty build metadata")
	}
	parts := strings.Split(value, ".")
	for _, part := range parts {
		if part == "" || !alphanumericHyphen(part) {
			return nil, errors.New("invalid build metadata")
		}
	}
	return parts, nil
}

// parseNumeric rejects empty input, non-digits, leading zeros, and values that
// do not fit in an unsigned 64-bit integer.
func parseNumeric(value string) (uint64, error) {
	if value == "" || (len(value) > 1 && value[0] == '0') {
		return 0, errors.New("invalid numeric identifier")
	}
	for index := 0; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return 0, errors.New("invalid numeric identifier")
		}
	}
	number, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, errors.New("numeric identifier out of range")
	}
	return number, nil
}

func numeric(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return value != ""
}

func alphanumericHyphen(value string) bool {
	for index := 0; index < len(value); index++ {
		byte := value[index]
		switch {
		case byte >= '0' && byte <= '9':
		case byte >= 'a' && byte <= 'z':
		case byte >= 'A' && byte <= 'Z':
		case byte == '-':
		default:
			return false
		}
	}
	return true
}
