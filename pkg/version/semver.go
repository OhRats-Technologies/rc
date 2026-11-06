package version

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Semver is a parsed semantic version. Only the fields RC needs are retained:
// build metadata is ignored for ordering and prerelease identifiers follow
// semver precedence rules.
type Semver struct {
	Major      uint64
	Minor      uint64
	Patch      uint64
	Prerelease []string
}

// ParseSemver parses MAJOR.MINOR.PATCH with an optional -prerelease. Build
// metadata after "+" is accepted and ignored.
func ParseSemver(value string) (Semver, error) {
	text := strings.TrimSpace(value)
	text = strings.TrimPrefix(text, "v")
	if idx := strings.IndexByte(text, '+'); idx >= 0 {
		text = text[:idx]
	}
	var pre []string
	if idx := strings.IndexByte(text, '-'); idx >= 0 {
		pre = strings.Split(text[idx+1:], ".")
		text = text[:idx]
		if len(pre) == 0 {
			return Semver{}, fmt.Errorf("invalid semantic version %q", value)
		}
		for _, part := range pre {
			if part == "" {
				return Semver{}, fmt.Errorf("invalid semantic version %q", value)
			}
		}
	}
	parts := strings.Split(text, ".")
	if len(parts) != 3 {
		return Semver{}, fmt.Errorf("invalid semantic version %q", value)
	}
	numbers := make([]uint64, 3)
	for i, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return Semver{}, fmt.Errorf("invalid semantic version %q", value)
		}
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return Semver{}, fmt.Errorf("invalid semantic version %q", value)
		}
		numbers[i] = n
	}
	return Semver{Major: numbers[0], Minor: numbers[1], Patch: numbers[2], Prerelease: pre}, nil
}

// Compare returns -1 when a < b, 0 when equal, and 1 when a > b.
func Compare(a, b Semver) int {
	if a.Major != b.Major {
		return compareUint(a.Major, b.Major)
	}
	if a.Minor != b.Minor {
		return compareUint(a.Minor, b.Minor)
	}
	if a.Patch != b.Patch {
		return compareUint(a.Patch, b.Patch)
	}
	return comparePrerelease(a.Prerelease, b.Prerelease)
}

// CompareStrings parses and compares two versions.
func CompareStrings(a, b string) (int, error) {
	left, err := ParseSemver(a)
	if err != nil {
		return 0, err
	}
	right, err := ParseSemver(b)
	if err != nil {
		return 0, err
	}
	return Compare(left, right), nil
}

// Newer reports whether candidate is strictly newer than current.
func Newer(candidate, current string) (bool, error) {
	order, err := CompareStrings(candidate, current)
	if err != nil {
		return false, err
	}
	return order > 0, nil
}

func compareUint(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func comparePrerelease(a, b []string) int {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	// A version without a prerelease sorts after one with a prerelease.
	if len(a) == 0 {
		return 1
	}
	if len(b) == 0 {
		return -1
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		left, leftErr := strconv.ParseUint(a[i], 10, 64)
		right, rightErr := strconv.ParseUint(b[i], 10, 64)
		switch {
		case leftErr == nil && rightErr == nil:
			if left != right {
				return compareUint(left, right)
			}
		case leftErr == nil:
			return -1 // numeric identifiers sort before alphanumeric
		case rightErr == nil:
			return 1
		default:
			if a[i] != b[i] {
				if a[i] < b[i] {
					return -1
				}
				return 1
			}
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	default:
		return 0
	}
}

// ErrEmptyVersion is returned when a release reports no version.
var ErrEmptyVersion = errors.New("empty version")
