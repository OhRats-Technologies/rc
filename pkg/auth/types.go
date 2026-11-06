package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// AlgorithmES256 is the COSE algorithm identifier stored for passkeys.
	AlgorithmES256 int64 = -7

	maxNameBytes       = 128
	maxCredentialBytes = 1023

	// SessionTokenBytes is the entropy of every issued session token.
	SessionTokenBytes = 32
)

// User is a human RC account.
type User struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

// Passkey is a registered WebAuthn credential.
type Passkey struct {
	ID           string
	UserID       string
	Name         string
	CredentialID []byte
	PublicKey    []byte
	Algorithm    int64
	CreatedAt    time.Time
	LastUsedAt   *time.Time
}

// Session is an active browser session backed by a hashed token.
type Session struct {
	UserID    string
	ExpiresAt time.Time
}

func normalizeName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if len(name) == 0 || len(name) > maxNameBytes || !utf8.ValidString(name) {
		return "", ErrInvalidName
	}
	return name, nil
}

func randomToken(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

// tokenHash validates the canonical base64url token encoding and returns the
// SHA-256 hash persisted in the sessions table.
func tokenHash(token string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(decoded) != SessionTokenBytes {
		return nil, ErrInvalidSessionToken
	}
	digest := sha256.Sum256(decoded)
	return digest[:], nil
}

func nowMs() int64 {
	return time.Now().UnixMilli()
}

func msFromTime(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UnixMilli()
}

func timeFromMs(value int64) time.Time {
	return time.UnixMilli(value).UTC()
}
