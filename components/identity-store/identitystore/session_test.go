package identitystore

import "testing"

const (
	firstSessionToken  = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	secondSessionToken = "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE"
)

func TestCookieTokenIgnoresMissingAndMalformedSessionCookies(t *testing.T) {
	if _, found := cookieToken("theme=dark"); found {
		t.Fatal("cookie without a session token was accepted")
	}
	if _, found := cookieToken("rc_session=not-base64"); found {
		t.Fatal("malformed session token was accepted")
	}
}

func TestCookieTokenRejectsDuplicateSessionCookies(t *testing.T) {
	header := "rc_session=" + firstSessionToken + "; rc_session=" + firstSessionToken
	if _, found := cookieToken(header); found {
		t.Fatal("duplicate session cookies were accepted for lookup")
	}
}

func TestCookieTokenAcceptsSingleSessionCookie(t *testing.T) {
	token, found := cookieToken("theme=dark; rc_session=" + firstSessionToken + "; lang=en")
	if !found || token != firstSessionToken {
		t.Fatalf("session token lookup = (%q, %t)", token, found)
	}
}

func TestCookieTokensCollectsEveryValidSessionCookie(t *testing.T) {
	header := "rc_session=" + firstSessionToken +
		"; rc_session=invalid; rc_session=" + secondSessionToken +
		"; rc_session=" + firstSessionToken
	tokens := cookieTokens(header)
	if len(tokens) != 2 || tokens[0] != firstSessionToken || tokens[1] != secondSessionToken {
		t.Fatalf("cookie tokens = %q", tokens)
	}
}

func TestCookieTokensIgnoresOversizedHeaders(t *testing.T) {
	header := "rc_session=" + firstSessionToken + ";"
	for len(header) <= maxCookieHeaderBytes {
		header += "x"
	}
	if tokens := cookieTokens(header); len(tokens) != 0 {
		t.Fatalf("oversized header produced %d tokens", len(tokens))
	}
	if _, found := cookieToken(header); found {
		t.Fatal("oversized header produced a session token")
	}
}
