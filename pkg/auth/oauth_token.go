package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"time"
)

// IssueOAuthTokens issues a resource-bound access token and refresh token.
func (s *Store) IssueOAuthTokens(
	clientID, userID, resource, scopes string,
) (string, string, time.Time, time.Time, error) {
	resource = strings.TrimSpace(resource)
	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(userID) == "" || resource == "" {
		return "", "", time.Time{}, time.Time{}, ErrOAuthInvalidGrant
	}
	if strings.TrimSpace(scopes) == "" {
		scopes = "mcp"
	}
	access, refresh, accessExpires, refreshExpires, err := newOAuthTokenPair()
	if err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}
	accessHash, ok := oauthTokenHash(access)
	if !ok {
		return "", "", time.Time{}, time.Time{}, ErrOAuthInvalidGrant
	}
	refreshHash, ok := oauthTokenHash(refresh)
	if !ok {
		return "", "", time.Time{}, time.Time{}, ErrOAuthInvalidGrant
	}
	_, err = s.db.ExecContext(
		context.Background(),
		`INSERT INTO oauth_tokens
		 (access_hash, refresh_hash, client_id, user_id, resource, scopes,
		  access_expires_ms, refresh_expires_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?);`,
		accessHash,
		refreshHash,
		clientID,
		userID,
		resource,
		scopes,
		msFromTime(accessExpires),
		msFromTime(refreshExpires),
	)
	if err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}
	return access, refresh, accessExpires, refreshExpires, nil
}

// ExchangeOAuthRefresh rotates a refresh token. The presented token is replaced
// atomically, so the previous refresh and access tokens stop working.
func (s *Store) ExchangeOAuthRefresh(refresh, clientID string) (string, string, time.Time, time.Time, error) {
	refreshHash, ok := oauthTokenHash(refresh)
	if !ok {
		return "", "", time.Time{}, time.Time{}, ErrOAuthInvalidGrant
	}
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var storedClient string
	var refreshExpiresMs int64
	var revoked sql.NullInt64
	err = tx.QueryRowContext(
		ctx,
		`SELECT client_id, refresh_expires_ms, revoked_ms
		 FROM oauth_tokens WHERE refresh_hash = ?;`,
		refreshHash,
	).Scan(&storedClient, &refreshExpiresMs, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", time.Time{}, time.Time{}, ErrOAuthInvalidGrant
	}
	if err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}
	if subtle.ConstantTimeCompare([]byte(storedClient), []byte(clientID)) != 1 {
		return "", "", time.Time{}, time.Time{}, ErrOAuthInvalidClient
	}
	if revoked.Valid {
		return "", "", time.Time{}, time.Time{}, ErrOAuthTokenRevoked
	}
	if refreshExpiresMs <= nowMs() {
		return "", "", time.Time{}, time.Time{}, ErrOAuthTokenExpired
	}

	access, newRefresh, accessExpires, refreshExpires, err := newOAuthTokenPair()
	if err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}
	accessHash, ok := oauthTokenHash(access)
	if !ok {
		return "", "", time.Time{}, time.Time{}, ErrOAuthInvalidGrant
	}
	newRefreshHash, ok := oauthTokenHash(newRefresh)
	if !ok {
		return "", "", time.Time{}, time.Time{}, ErrOAuthInvalidGrant
	}
	result, err := tx.ExecContext(
		ctx,
		`UPDATE oauth_tokens
		 SET access_hash = ?, refresh_hash = ?, access_expires_ms = ?, refresh_expires_ms = ?
		 WHERE refresh_hash = ? AND revoked_ms IS NULL;`,
		accessHash,
		newRefreshHash,
		msFromTime(accessExpires),
		msFromTime(refreshExpires),
		refreshHash,
	)
	if err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}
	if affected != 1 {
		return "", "", time.Time{}, time.Time{}, ErrOAuthInvalidGrant
	}
	if err := tx.Commit(); err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}
	return access, newRefresh, accessExpires, refreshExpires, nil
}

// LookupOAuthAccess validates an access token and its resource binding.
func (s *Store) LookupOAuthAccess(token, resource string) (*OAuthGrant, error) {
	hash, ok := oauthTokenHash(token)
	if !ok {
		return nil, ErrOAuthTokenNotFound
	}
	var grant OAuthGrant
	var accessExpiresMs, refreshExpiresMs int64
	var revoked sql.NullInt64
	err := s.db.QueryRowContext(
		context.Background(),
		`SELECT client_id, user_id, resource, scopes, access_expires_ms, refresh_expires_ms, revoked_ms
		 FROM oauth_tokens WHERE access_hash = ?;`,
		hash,
	).Scan(
		&grant.ClientID,
		&grant.UserID,
		&grant.Resource,
		&grant.Scopes,
		&accessExpiresMs,
		&refreshExpiresMs,
		&revoked,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOAuthTokenNotFound
	}
	if err != nil {
		return nil, err
	}
	if revoked.Valid {
		return nil, ErrOAuthTokenRevoked
	}
	if accessExpiresMs <= nowMs() {
		return nil, ErrOAuthTokenExpired
	}
	if resource != "" && grant.Resource != resource {
		return nil, ErrOAuthResourceMismatch
	}
	grant.AccessExpiresAt = timeFromMs(accessExpiresMs)
	grant.RefreshExpiresAt = timeFromMs(refreshExpiresMs)
	return &grant, nil
}

func newOAuthTokenPair() (string, string, time.Time, time.Time, error) {
	access, err := randomToken(OAuthTokenBytes)
	if err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}
	refresh, err := randomToken(OAuthTokenBytes)
	if err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}
	now := time.Now().UTC()
	return access, refresh, now.Add(OAuthAccessTokenLifetime), now.Add(OAuthRefreshTokenLifetime), nil
}

// oauthTokenHash validates the canonical base64url encoding of an OAuth secret
// and returns the SHA-256 digest persisted in SQLite.
func oauthTokenHash(token string) ([]byte, bool) {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(decoded) != OAuthTokenBytes {
		return nil, false
	}
	digest := sha256.Sum256(decoded)
	return digest[:], true
}
