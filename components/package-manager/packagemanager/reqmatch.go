package packagemanager

import "strings"

// Matches reports whether the version satisfies the requirement.
func (requirement VersionReq) Matches(version Version) bool {
	for _, comparator := range requirement.Comparators {
		if !comparator.matches(version) {
			return false
		}
	}
	if version.Pre == "" {
		return true
	}
	for _, comparator := range requirement.Comparators {
		if comparator.prereleaseCompatible(version) {
			return true
		}
	}
	return false
}

func (comparator Comparator) matches(version Version) bool {
	switch comparator.Op {
	case opExact, opWildcard:
		return comparator.matchesExact(version)
	case opGreater:
		return comparator.matchesGreater(version)
	case opGreaterEq:
		return comparator.matchesExact(version) || comparator.matchesGreater(version)
	case opLess:
		return comparator.matchesLess(version)
	case opLessEq:
		return comparator.matchesExact(version) || comparator.matchesLess(version)
	case opTilde:
		return comparator.matchesTilde(version)
	case opCaret:
		return comparator.matchesCaret(version)
	default:
		return false
	}
}

func (comparator Comparator) matchesExact(version Version) bool {
	if version.Major != comparator.Major {
		return false
	}
	if comparator.Minor != nil && version.Minor != *comparator.Minor {
		return false
	}
	if comparator.Patch != nil && version.Patch != *comparator.Patch {
		return false
	}
	return version.Pre == comparator.Pre
}

func (comparator Comparator) matchesGreater(version Version) bool {
	if version.Major != comparator.Major {
		return version.Major > comparator.Major
	}
	if comparator.Minor == nil {
		return false
	}
	if version.Minor != *comparator.Minor {
		return version.Minor > *comparator.Minor
	}
	if comparator.Patch == nil {
		return false
	}
	if version.Patch != *comparator.Patch {
		return version.Patch > *comparator.Patch
	}
	return comparePrerelease(version.Pre, comparator.Pre) > 0
}

func (comparator Comparator) matchesLess(version Version) bool {
	if version.Major != comparator.Major {
		return version.Major < comparator.Major
	}
	if comparator.Minor == nil {
		return false
	}
	if version.Minor != *comparator.Minor {
		return version.Minor < *comparator.Minor
	}
	if comparator.Patch == nil {
		return false
	}
	if version.Patch != *comparator.Patch {
		return version.Patch < *comparator.Patch
	}
	return comparePrerelease(version.Pre, comparator.Pre) < 0
}

func (comparator Comparator) matchesTilde(version Version) bool {
	if version.Major != comparator.Major {
		return false
	}
	if comparator.Minor != nil && version.Minor != *comparator.Minor {
		return false
	}
	if comparator.Patch != nil && version.Patch != *comparator.Patch {
		return version.Patch > *comparator.Patch
	}
	return comparePrerelease(version.Pre, comparator.Pre) >= 0
}

func (comparator Comparator) matchesCaret(version Version) bool {
	if version.Major != comparator.Major {
		return false
	}
	if comparator.Minor == nil {
		return true
	}
	minor := *comparator.Minor
	if comparator.Patch == nil {
		if comparator.Major > 0 {
			return version.Minor >= minor
		}
		return version.Minor == minor
	}
	patch := *comparator.Patch
	if comparator.Major > 0 {
		if version.Minor != minor {
			return version.Minor > minor
		}
		if version.Patch != patch {
			return version.Patch > patch
		}
	} else if minor > 0 {
		if version.Minor != minor {
			return false
		}
		if version.Patch != patch {
			return version.Patch > patch
		}
	} else if version.Minor != minor || version.Patch != patch {
		return false
	}
	return comparePrerelease(version.Pre, comparator.Pre) >= 0
}

func (comparator Comparator) prereleaseCompatible(version Version) bool {
	return comparator.Major == version.Major &&
		comparator.Minor != nil && *comparator.Minor == version.Minor &&
		comparator.Patch != nil && *comparator.Patch == version.Patch &&
		comparator.Pre != ""
}

func (requirement VersionReq) String() string {
	if len(requirement.Comparators) == 0 {
		return "*"
	}
	parts := make([]string, 0, len(requirement.Comparators))
	for _, comparator := range requirement.Comparators {
		parts = append(parts, comparator.String())
	}
	return strings.Join(parts, ", ")
}

func (comparator Comparator) String() string {
	result := ""
	switch comparator.Op {
	case opExact:
		result = "="
	case opGreater:
		result = ">"
	case opGreaterEq:
		result = ">="
	case opLess:
		result = "<"
	case opLessEq:
		result = "<="
	case opTilde:
		result = "~"
	case opCaret:
		result = "^"
	case opWildcard:
		result = ""
	}
	result += formatUint(comparator.Major)
	if comparator.Minor != nil {
		result += "." + formatUint(*comparator.Minor)
		if comparator.Patch != nil {
			result += "." + formatUint(*comparator.Patch)
			if comparator.Pre != "" {
				result += "-" + comparator.Pre
			}
		} else if comparator.Op == opWildcard {
			result += ".*"
		}
	} else if comparator.Op == opWildcard {
		result += ".*"
	}
	return result
}
