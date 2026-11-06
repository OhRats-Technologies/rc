package auth

import (
	"context"
	"errors"
	"time"
)

// ErrEnrollmentToken is returned when a one-time enrollment token is invalid.
var ErrEnrollmentToken = errors.New("invalid or expired enrollment token")

// CreateEnrollmentToken issues a one-time workspace enrollment token. Only its
// SHA-256 hash is stored; the plaintext is returned once.
func (s *Store) CreateEnrollmentToken(workspaceID string, lifetime time.Duration) (string, time.Time, error) {
	if workspaceID == "" || lifetime <= 0 || lifetime > 30*24*time.Hour {
		return "", time.Time{}, errors.New("invalid enrollment token request")
	}
	token, err := randomToken(32)
	if err != nil {
		return "", time.Time{}, err
	}
	hash, err := tokenHash(token)
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().Add(lifetime)
	if _, err := s.db.ExecContext(context.Background(),
		`INSERT INTO enrollment_tokens (token_hash, workspace_id, expires_ms) VALUES (?, ?, ?)`,
		hash, workspaceID, msFromTime(expires)); err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// ConsumeEnrollmentToken atomically consumes a token and returns its workspace.
func (s *Store) ConsumeEnrollmentToken(token string) (string, error) {
	if token == "" {
		return "", ErrEnrollmentToken
	}
	hash, err := tokenHash(token)
	if err != nil {
		return "", ErrEnrollmentToken
	}
	var workspaceID string
	err = s.db.QueryRowContext(context.Background(),
		`DELETE FROM enrollment_tokens WHERE token_hash = ? AND expires_ms > ? RETURNING workspace_id`,
		hash, nowMs()).Scan(&workspaceID)
	if err != nil {
		return "", ErrEnrollmentToken
	}
	return workspaceID, nil
}
