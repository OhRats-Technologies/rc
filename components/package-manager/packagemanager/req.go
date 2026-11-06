package packagemanager

import (
	"strings"
	"unicode/utf8"
)

type versionOp int

const (
	opExact versionOp = iota
	opGreater
	opGreaterEq
	opLess
	opLessEq
	opTilde
	opCaret
	opWildcard
)

// Comparator is one semver requirement comparator.
type Comparator struct {
	Op    versionOp
	Major uint64
	Minor *uint64
	Patch *uint64
	Pre   string
}

// VersionReq matches versions using the Rust `semver` crate rules. An empty
// comparator list is the STAR requirement, which is what "*" parses to.
type VersionReq struct {
	Comparators []Comparator
}

// ParseVersionReq parses comma separated comparators.
func ParseVersionReq(text string) (VersionReq, error) {
	text = strings.TrimLeft(text, " ")
	if wildcardChar, rest, ok := stripWildcard(text); ok {
		rest = strings.TrimLeft(rest, " ")
		switch {
		case rest == "":
			return VersionReq{}, nil
		case strings.HasPrefix(rest, ","):
			return VersionReq{}, semverErrorf(
				"wildcard req (%c) must be the only comparator in the version req",
				wildcardChar,
			)
		default:
			return VersionReq{}, semverError("unexpected character after wildcard in version req")
		}
	}
	var comparators []Comparator
	for {
		comparator, position, rest, err := parseComparator(text)
		if err != nil {
			if wildcardChar, after, ok := stripWildcard(text); ok {
				after = strings.TrimLeft(after, " ")
				if after == "" || strings.HasPrefix(after, ",") {
					err = semverErrorf(
						"wildcard req (%c) must be the only comparator in the version req",
						wildcardChar,
					)
				}
			}
			return VersionReq{}, err
		}
		comparators = append(comparators, comparator)
		if rest == "" {
			break
		}
		if !strings.HasPrefix(rest, ",") {
			unexpected, _ := utf8.DecodeRuneInString(rest)
			return VersionReq{}, semverErrorf(
				"expected comma after %s, found %s",
				position,
				quotedChar(unexpected),
			)
		}
		if len(comparators) >= 32 {
			return VersionReq{}, semverError("excessive number of version comparators")
		}
		text = strings.TrimLeft(rest[1:], " ")
	}
	return VersionReq{Comparators: comparators}, nil
}

func stripWildcard(text string) (rune, string, bool) {
	if text == "" {
		return 0, text, false
	}
	switch text[0] {
	case '*':
		return '*', text[1:], true
	case 'x':
		return 'x', text[1:], true
	case 'X':
		return 'X', text[1:], true
	default:
		return 0, text, false
	}
}

func parseComparator(text string) (Comparator, string, string, error) {
	op := opCaret
	defaultOp := true
	rest := text
	switch {
	case strings.HasPrefix(rest, "="):
		op, rest, defaultOp = opExact, rest[1:], false
	case strings.HasPrefix(rest, ">="):
		op, rest, defaultOp = opGreaterEq, rest[2:], false
	case strings.HasPrefix(rest, ">"):
		op, rest, defaultOp = opGreater, rest[1:], false
	case strings.HasPrefix(rest, "<="):
		op, rest, defaultOp = opLessEq, rest[2:], false
	case strings.HasPrefix(rest, "<"):
		op, rest, defaultOp = opLess, rest[1:], false
	case strings.HasPrefix(rest, "~"):
		op, rest, defaultOp = opTilde, rest[1:], false
	case strings.HasPrefix(rest, "^"):
		op, rest, defaultOp = opCaret, rest[1:], false
	}
	rest = strings.TrimLeft(rest, " ")

	position := "major version number"
	major, rest, err := numericIdentifier(rest, position)
	if err != nil {
		return Comparator{}, "", "", err
	}
	var minor *uint64
	var patch *uint64
	hasWildcard := false
	if strings.HasPrefix(rest, ".") {
		position = "minor version number"
		if _, after, ok := stripWildcard(rest[1:]); ok {
			hasWildcard = true
			if defaultOp {
				op = opWildcard
			}
			rest = after
		} else {
			value, after, err := numericIdentifier(rest[1:], position)
			if err != nil {
				return Comparator{}, "", "", err
			}
			minor = &value
			rest = after
		}
	}
	if strings.HasPrefix(rest, ".") {
		position = "patch version number"
		if _, after, ok := stripWildcard(rest[1:]); ok {
			if defaultOp {
				op = opWildcard
			}
			rest = after
		} else if hasWildcard {
			return Comparator{}, "", "", semverError("unexpected character after wildcard in version req")
		} else {
			value, after, err := numericIdentifier(rest[1:], position)
			if err != nil {
				return Comparator{}, "", "", err
			}
			patch = &value
			rest = after
		}
	}
	pre := ""
	if patch != nil && strings.HasPrefix(rest, "-") {
		position = "pre-release identifier"
		value, after, err := identifier(rest[1:], position)
		if err != nil {
			return Comparator{}, "", "", err
		}
		if value == "" {
			return Comparator{}, "", "", semverErrorf("empty identifier segment in %s", position)
		}
		pre = value
		rest = after
	}
	if patch != nil && strings.HasPrefix(rest, "+") {
		position = "build metadata"
		value, after, err := identifier(rest[1:], position)
		if err != nil {
			return Comparator{}, "", "", err
		}
		if value == "" {
			return Comparator{}, "", "", semverErrorf("empty identifier segment in %s", position)
		}
		rest = after
	}
	rest = strings.TrimLeft(rest, " ")
	return Comparator{Op: op, Major: major, Minor: minor, Patch: patch, Pre: pre}, position, rest, nil
}
