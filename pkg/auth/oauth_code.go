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

// IssueOAuthCode creates a single-use authorization code bound to the client,
// redirect URI, PKCE challenge, user, and resource. Only its hash is stored.
func (s *Store) IssueOAuthCode(clientID, userID, redirectURI, codeChallenge, resource string) (string, error) {
	if err := s.ValidateRedirectURI(clientID, redirectURI); err != nil {
		return "", err
	}
	if strings.TrimSpace(userID) == "" {
		return "", ErrUserNotFound
	}
	if _, err := s.GetUser(userID); err != nil {
		return "", err
	}
	challenge := strings.TrimSpace(codeChallenge)
	resource = strings.TrimSpace(resource)
	if challenge == "" || resource == "" {
		return "", ErrOAuthInvalidGrant
	}
	code, err := randomToken(OAuthTokenBytes)
	if err != nil {
		return "", err
	}
	hash, ok := oauthTokenHash(code)
	if !ok {
		return "", ErrOAuthInvalidCode
	}
	_, err = s.db.ExecContext(
		context.Background(),
		`INSERT INTO oauth_codes
		 (code_hash, client_id, user_id, redirect_uri, code_challenge, resource, expires_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?);`,
		hash,
		clientID,
		userID,
		redirectURI,
		challenge,
		resource,
		msFromTime(time.Now().Add(OAuthCodeLifetime)),
	)
	if err != nil {
		return "", err
	}
	return code, nil
}

// ConsumeOAuthCode atomically redeems an authorization code and verifies its
// PKCE S256 verifier. A code is deleted even when verification fails.
func (s *Store) ConsumeOAuthCode(code, clientID, redirectURI, verifier string) (string, string, error) {
	hash, ok := oauthTokenHash(code)
	if !ok {
		return "", "", ErrOAuthInvalidCode
	}
	var userID, storedClient, storedRedirect, challenge, resource string
	err := s.db.QueryRowContext(
		context.Background(),
		`DELETE FROM oauth_codes WHERE code_hash = ? AND expires_ms > ?
		 RETURNING user_id, client_id, redirect_uri, code_challenge, resource;`,
		hash,
		nowMs(),
	).Scan(&userID, &storedClient, &storedRedirect, &challenge, &resource)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrOAuthInvalidCode
	}
	if err != nil {
		return "", "", err
	}
	if subtle.ConstantTimeCompare([]byte(storedClient), []byte(clientID)) != 1 ||
		subtle.ConstantTimeCompare([]byte(storedRedirect), []byte(redirectURI)) != 1 {
		return "", "", ErrOAuthInvalidGrant
	}
	digest := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(digest[:])
	if subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) != 1 {
		return "", "", ErrOAuthInvalidVerifier
	}
	return userID, resource, nil
}
