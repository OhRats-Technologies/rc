package packagemanager

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Version is a semver 2.0 version with the same accepted grammar, error
// messages and precedence rules as the Rust `semver` crate.
type Version struct {
	Major uint64
	Minor uint64
	Patch uint64
	Pre   string
	Build string
}

func semverError(message string) error {
	return errors.New(message)
}

func semverErrorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

// ParseVersion parses "major.minor.patch[-pre][+build]".
func ParseVersion(text string) (Version, error) {
	if text == "" {
		return Version{}, semverError("empty string, expected a semver version")
	}
	major, rest, err := numericIdentifier(text, "major version number")
	if err != nil {
		return Version{}, err
	}
	if rest, err = expectDot(rest, "major version number"); err != nil {
		return Version{}, err
	}
	minor, rest, err := numericIdentifier(rest, "minor version number")
	if err != nil {
		return Version{}, err
	}
	if rest, err = expectDot(rest, "minor version number"); err != nil {
		return Version{}, err
	}
	patch, rest, err := numericIdentifier(rest, "patch version number")
	if err != nil {
		return Version{}, err
	}

	value := Version{Major: major, Minor: minor, Patch: patch}
	position := "patch version number"
	if strings.HasPrefix(rest, "-") {
		position = "pre-release identifier"
		pre, text, err := identifier(rest[1:], position)
		if err != nil {
			return Version{}, err
		}
		if pre == "" {
			return Version{}, semverErrorf("empty identifier segment in %s", position)
		}
		value.Pre = pre
		rest = text
	}
	if strings.HasPrefix(rest, "+") {
		position = "build metadata"
		build, text, err := identifier(rest[1:], position)
		if err != nil {
			return Version{}, err
		}
		if build == "" {
			return Version{}, semverErrorf("empty identifier segment in %s", position)
		}
		value.Build = build
		rest = text
	}
	if rest != "" {
		r, _ := utf8.DecodeRuneInString(rest)
		return Version{}, semverErrorf("unexpected character %s after %s", quotedChar(r), position)
	}
	return value, nil
}

func numericIdentifier(text string, position string) (uint64, string, error) {
	var value uint64
	length := 0
	for length < len(text) {
		digit := text[length]
		if digit < '0' || digit > '9' {
			break
		}
		if value == 0 && length > 0 {
			return 0, "", semverErrorf("invalid leading zero in %s", position)
		}
		next := uint64(digit - '0')
		if value > (math.MaxUint64-next)/10 {
			return 0, "", semverErrorf("value of %s exceeds u64::MAX", position)
		}
		value = value*10 + next
		length++
	}
	if length > 0 {
		return value, text[length:], nil
	}
	if length < len(text) {
		r, _ := utf8.DecodeRuneInString(text)
		return 0, "", semverErrorf("unexpected character %s while parsing %s", quotedChar(r), position)
	}
	return 0, "", semverErrorf("unexpected end of input while parsing %s", position)
}

func expectDot(text string, position string) (string, error) {
	if strings.HasPrefix(text, ".") {
		return text[1:], nil
	}
	if text == "" {
		return "", semverErrorf("unexpected end of input while parsing %s", position)
	}
	r, _ := utf8.DecodeRuneInString(text)
	return "", semverErrorf("unexpected character %s after %s", quotedChar(r), position)
}

func identifier(text string, position string) (string, string, error) {
	accumulated := 0
	segment := 0
	nondigit := false
	for {
		if accumulated+segment >= len(text) {
			if segment == 0 {
				if accumulated == 0 {
					return "", text, nil
				}
				return "", "", semverErrorf("empty identifier segment in %s", position)
			}
			if leadingNumericSegment(text, accumulated, segment, nondigit, position) {
				return "", "", semverErrorf("invalid leading zero in %s", position)
			}
			accumulated += segment
			return text[:accumulated], text[accumulated:], nil
		}
		value := text[accumulated+segment]
		switch {
		case isAsciiLetter(value) || value == '-':
			segment++
			nondigit = true
		case value >= '0' && value <= '9':
			segment++
		default:
			if segment == 0 {
				if accumulated == 0 && value != '.' {
					return "", text, nil
				}
				return "", "", semverErrorf("empty identifier segment in %s", position)
			}
			if leadingNumericSegment(text, accumulated, segment, nondigit, position) {
				return "", "", semverErrorf("invalid leading zero in %s", position)
			}
			accumulated += segment
			if value == '.' {
				accumulated++
				segment = 0
				nondigit = false
				continue
			}
			return text[:accumulated], text[accumulated:], nil
		}
	}
}

func leadingNumericSegment(text string, accumulated int, segment int, nondigit bool, position string) bool {
	return position == "pre-release identifier" && segment > 1 && !nondigit && text[accumulated] == '0'
}

func isAsciiLetter(value byte) bool {
	return (value >= 'A' && value <= 'Z') || (value >= 'a' && value <= 'z')
}

func (value Version) String() string {
	result := strconv.FormatUint(value.Major, 10) + "." +
		strconv.FormatUint(value.Minor, 10) + "." +
		strconv.FormatUint(value.Patch, 10)
	if value.Pre != "" {
		result += "-" + value.Pre
	}
	if value.Build != "" {
		result += "+" + value.Build
	}
	return result
}

// Compare orders versions by semver precedence, ignoring build metadata.
func (value Version) Compare(other Version) int {
	if result := compareUint(value.Major, other.Major); result != 0 {
		return result
	}
	if result := compareUint(value.Minor, other.Minor); result != 0 {
		return result
	}
	if result := compareUint(value.Patch, other.Patch); result != 0 {
		return result
	}
	return comparePrerelease(value.Pre, other.Pre)
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

func comparePrerelease(left string, right string) int {
	if left == "" && right == "" {
		return 0
	}
	// A real release compares greater than any prerelease.
	if left == "" {
		return 1
	}
	if right == "" {
		return -1
	}
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	for index := 0; index < len(leftParts) && index < len(rightParts); index++ {
		leftPart := leftParts[index]
		rightPart := rightParts[index]
		leftNumeric := allDigits(leftPart)
		rightNumeric := allDigits(rightPart)
		var result int
		switch {
		case leftNumeric && rightNumeric:
			result = compareNumericString(leftPart, rightPart)
		case leftNumeric:
			result = -1
		case rightNumeric:
			result = 1
		default:
			result = strings.Compare(leftPart, rightPart)
		}
		if result != 0 {
			return result
		}
	}
	switch {
	case len(leftParts) < len(rightParts):
		return -1
	case len(leftParts) > len(rightParts):
		return 1
	default:
		return 0
	}
}

func allDigits(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}

func compareNumericString(left string, right string) int {
	left = strings.TrimLeft(left, "0")
	right = strings.TrimLeft(right, "0")
	switch {
	case len(left) < len(right):
		return -1
	case len(left) > len(right):
		return 1
	default:
		return strings.Compare(left, right)
	}
}
