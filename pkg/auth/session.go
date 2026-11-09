package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// DefaultSessionLifetime is used when no lifetime is requested.
const DefaultSessionLifetime = 30 * 24 * time.Hour

// IssueSession creates a browser session and returns the plaintext token with
// its expiry. Only the SHA-256 hash of the token is stored.
func (s *Store) IssueSession(userID string, lifetime string) (string, time.Time, error) {
	if userID == "" {
		return "", time.Time{}, ErrUserNotFound
	}
	duration, err := sessionLifetime(lifetime)
	if err != nil {
		return "", time.Time{}, err
	}
	token, err := randomToken(SessionTokenBytes)
	if err != nil {
		return "", time.Time{}, err
	}
	hash, err := tokenHash(token)
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().Add(duration).UTC()
	_, err = s.db.ExecContext(
		context.Background(),
		`INSERT INTO sessions (token_hash, user_id, expires_ms) VALUES (?, ?, ?);`,
		hash,
		userID,
		msFromTime(expiresAt),
	)
	if err != nil {
		if isConstraint(err, sqliteConstraintForeignKey) {
			return "", time.Time{}, ErrUserNotFound
		}
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

// LookupSession resolves a token to its account. Expired sessions are removed
// and reported as expired.
func (s *Store) LookupSession(token string) (*Session, error) {
	hash, err := tokenHash(token)
	if err != nil {
		return nil, err
	}
	var userID string
	var expiresMs int64
	err = s.db.QueryRowContext(
		context.Background(),
		`SELECT user_id, expires_ms FROM sessions WHERE token_hash = ?;`,
		hash,
	).Scan(&userID, &expiresMs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	expiresAt := timeFromMs(expiresMs)
	if !expiresAt.After(time.Now()) {
		_, _ = s.db.ExecContext(
			context.Background(),
			`DELETE FROM sessions WHERE token_hash = ?;`,
			hash,
		)
		return nil, ErrSessionExpired
	}
	return &Session{UserID: userID, ExpiresAt: expiresAt}, nil
}

// RevokeSession deletes a session. Revoking an unknown or malformed token is
// not an error, so logout stays idempotent.
func (s *Store) RevokeSession(token string) error {
	hash, err := tokenHash(token)
	if err != nil {
		return nil
	}
	_, err = s.db.ExecContext(
		context.Background(),
		`DELETE FROM sessions WHERE token_hash = ?;`,
		hash,
	)
	return err
}

// PruneExpired removes expired ceremonies, sessions, OAuth consent requests,
// OAuth codes, and OAuth token families.
func (s *Store) PruneExpired(now time.Time) (int, error) {
	ctx := context.Background()
	cutoff := msFromTime(now)
	removed := 0
	for _, statement := range []string{
		`DELETE FROM ceremonies WHERE expires_ms <= ?;`,
		`DELETE FROM sessions WHERE expires_ms <= ?;`,
		`DELETE FROM oauth_codes WHERE expires_ms <= ?;`,
		`DELETE FROM oauth_requests WHERE expires_ms <= ?;`,
		`DELETE FROM oauth_tokens WHERE refresh_expires_ms <= ?;`,
	} {
		result, err := s.db.ExecContext(ctx, statement, cutoff)
		if err != nil {
			return removed, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return removed, err
		}
		removed += int(affected)
	}
	return removed, nil
}

func sessionLifetime(value string) (time.Duration, error) {
	switch value {
	case "":
		return DefaultSessionLifetime, nil
	case "30d":
		return 30 * 24 * time.Hour, nil
	case "1d":
		return 24 * time.Hour, nil
	case "1h":
		return time.Hour, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, ErrInvalidLifetime
	}
	return duration, nil
}
