package auth

import "testing"

func TestOAuthScopeNormalization(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"", OAuthScopeObserve},
		{"mcp:observe", OAuthScopeObserve},
		{"mcp:terminal", OAuthScopeTerminal},
		{"mcp:terminal mcp:observe", OAuthScopeObserve + " " + OAuthScopeTerminal},
		{"mcp", OAuthScopeObserve + " " + OAuthScopeTerminal},
	}
	for _, tc := range cases {
		scopes, err := NormalizeOAuthScopes(tc.raw)
		if err != nil {
			t.Fatalf("normalize %q: %v", tc.raw, err)
		}
		if got := ScopeString(scopes); got != tc.want {
			t.Fatalf("normalize %q = %q, want %q", tc.raw, got, tc.want)
		}
	}
	if _, err := NormalizeOAuthScopes("admin"); err == nil {
		t.Fatal("unknown scope must be rejected")
	}
	if !HasOAuthScope(OAuthScopeTerminal, OAuthScopeObserve) {
		t.Fatal("terminal grant must imply observe")
	}
	if !HasOAuthScope(LegacyOAuthScope, OAuthScopeTerminal) {
		t.Fatal("legacy mcp scope must authorize terminal")
	}
	if HasOAuthScope(OAuthScopeObserve, OAuthScopeTerminal) {
		t.Fatal("observe grant must not authorize terminal")
	}
}
