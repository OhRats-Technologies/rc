package identitystore

import (
	"encoding/base64"
	"errors"
	"strings"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_session_types"
)

const (
	sessionBucket        = "browser-sessions"
	sessionCookie        = "rc_session"
	sessionTokenBytes    = 32
	sessionIDBytes       = 16
	maxCookieHeaderBytes = 8192
)

type storedSession struct {
	Id          string `json:"id"`
	UserId      string `json:"user_id"`
	CreatedAtMs uint64 `json:"created_at_ms"`
	ExpiresAtMs uint64 `json:"expires_at_ms"`
}

func IssueSession(userId string, expiresAtMs uint64) (ohrats_rc_session_types.IssuedSession, error) {
	principal, found, err := getUser(userId)
	if err != nil {
		return ohrats_rc_session_types.IssuedSession{}, err
	}
	if !found {
		return ohrats_rc_session_types.IssuedSession{}, errors.New("user not found")
	}
	createdAtMs := nowMs()
	if expiresAtMs != 0 && expiresAtMs <= createdAtMs {
		return ohrats_rc_session_types.IssuedSession{}, errors.New("browser session expiration must be in the future")
	}
	token, err := randomToken(sessionTokenBytes)
	if err != nil {
		return ohrats_rc_session_types.IssuedSession{}, err
	}
	id, err := randomToken(sessionIDBytes)
	if err != nil {
		return ohrats_rc_session_types.IssuedSession{}, err
	}
	stored := storedSession{
		Id:          id,
		UserId:      userId,
		CreatedAtMs: createdAtMs,
		ExpiresAtMs: expiresAtMs,
	}
	encoded, err := encode(stored)
	if err != nil {
		return ohrats_rc_session_types.IssuedSession{}, err
	}
	if err := insert(sessionBucket, tokenKey(token), encoded); err != nil {
		return ohrats_rc_session_types.IssuedSession{}, err
	}
	return ohrats_rc_session_types.IssuedSession{
		Token:   token,
		Session: sessionWire(stored, principal.DisplayName),
	}, nil
}

func FindSession(cookieHeader string) (witTypes.Option[ohrats_rc_session_types.Session], error) {
	token, ok := cookieToken(cookieHeader)
	if !ok {
		return witTypes.None[ohrats_rc_session_types.Session](), nil
	}
	key := tokenKey(token)
	value, found, err := getValue(sessionBucket, key)
	if err != nil || !found {
		return witTypes.None[ohrats_rc_session_types.Session](), err
	}
	stored, err := decode[storedSession](value)
	if err != nil {
		return witTypes.None[ohrats_rc_session_types.Session](), err
	}
	if sessionExpired(stored) {
		if _, err := removeValue(sessionBucket, key); err != nil {
			return witTypes.None[ohrats_rc_session_types.Session](), err
		}
		return witTypes.None[ohrats_rc_session_types.Session](), nil
	}
	principal, found, err := getUser(stored.UserId)
	if err != nil {
		return witTypes.None[ohrats_rc_session_types.Session](), err
	}
	if !found {
		if _, err := removeValue(sessionBucket, key); err != nil {
			return witTypes.None[ohrats_rc_session_types.Session](), err
		}
		return witTypes.None[ohrats_rc_session_types.Session](), nil
	}
	return witTypes.Some(sessionWire(stored, principal.DisplayName)), nil
}

func RevokeSession(cookieHeader string) (bool, error) {
	revoked := false
	for _, token := range cookieTokens(cookieHeader) {
		found, err := removeValue(sessionBucket, tokenKey(token))
		if err != nil {
			return false, err
		}
		revoked = revoked || found
	}
	return revoked, nil
}

func sessionWire(value storedSession, displayName string) ohrats_rc_session_types.Session {
	return ohrats_rc_session_types.Session{
		Id: value.Id,
		Principal: ohrats_rc_session_types.Principal{
			UserId:      value.UserId,
			DisplayName: displayName,
		},
		CreatedAtMs: value.CreatedAtMs,
		ExpiresAtMs: value.ExpiresAtMs,
	}
}

func sessionExpired(value storedSession) bool {
	return value.ExpiresAtMs != 0 && value.ExpiresAtMs <= nowMs()
}

// cookieToken returns the single session token of a cookie header. Duplicate or
// malformed session cookies are rejected instead of guessed.
func cookieToken(header string) (string, bool) {
	if len(header) > maxCookieHeaderBytes {
		return "", false
	}
	found := false
	token := ""
	for _, part := range strings.Split(header, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || name != sessionCookie {
			continue
		}
		if found || !validToken(value) {
			return "", false
		}
		found = true
		token = value
	}
	return token, found
}

func cookieTokens(header string) []string {
	if len(header) > maxCookieHeaderBytes {
		return nil
	}
	tokens := make([]string, 0)
	for _, part := range strings.Split(header, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || name != sessionCookie || !validToken(value) {
			continue
		}
		if slicesContains(tokens, value) {
			continue
		}
		tokens = append(tokens, value)
	}
	return tokens
}

func slicesContains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func validToken(value string) bool {
	if len(value) > 64 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == sessionTokenBytes
}

func tokenKey(token string) []byte {
	return digest([]byte(token))
}
