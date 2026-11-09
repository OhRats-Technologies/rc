package auth

import (
	"errors"
	"strings"
)

const (
	// OAuthScopeObserve allows read-only machine and process observation.
	OAuthScopeObserve = "mcp:observe"
	// OAuthScopeTerminal allows execution-capable process operations.
	OAuthScopeTerminal = "mcp:terminal"

	// LegacyOAuthScope is the pre-scope MCP grant. It is accepted for existing
	// clients and tokens and expands to both current scopes.
	LegacyOAuthScope = "mcp"
)

// ErrOAuthInvalidScope reports a scope that this authorization server does not
// support.
var ErrOAuthInvalidScope = errors.New("auth: unsupported oauth scope")

// NormalizeOAuthScopes parses a space-delimited scope value, rejects unknown
// scopes, and returns canonical order. An empty value requests observe only.
func NormalizeOAuthScopes(raw string) ([]string, error) {
	requested := strings.Fields(raw)
	if len(requested) == 0 {
		return []string{OAuthScopeObserve}, nil
	}
	observe, terminal := false, false
	for _, scope := range requested {
		switch scope {
		case OAuthScopeObserve:
			observe = true
		case OAuthScopeTerminal:
			terminal = true
		case LegacyOAuthScope:
			observe, terminal = true, true
		default:
			return nil, ErrOAuthInvalidScope
		}
	}
	scopes := make([]string, 0, 2)
	if observe {
		scopes = append(scopes, OAuthScopeObserve)
	}
	if terminal {
		scopes = append(scopes, OAuthScopeTerminal)
	}
	return scopes, nil
}

// ScopeString joins canonical scopes with the OAuth space delimiter.
func ScopeString(scopes []string) string {
	return strings.Join(scopes, " ")
}

// HasOAuthScope reports whether a stored scope value authorizes required.
// Observe is implied by a terminal grant. The legacy "mcp" scope authorizes
// both current scopes so previously issued tokens keep their meaning.
func HasOAuthScope(stored, required string) bool {
	for _, scope := range strings.Fields(stored) {
		if scope == LegacyOAuthScope || scope == required {
			return true
		}
		if required == OAuthScopeObserve && scope == OAuthScopeTerminal {
			return true
		}
	}
	return false
}
