package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// OAuthRequest is an authorization request awaiting explicit user consent.
type OAuthRequest struct {
	ID            string
	ClientID      string
	UserID        string
	RedirectURI   string
	State         string
	CodeChallenge string
	Resource      string
	Scopes        []string
	PreparedGrant string
	ExpiresAt     time.Time
}

// CreateOAuthRequest stores a browser consent request. The ID is returned to
// the consent page and is not a credential by itself: every lookup also
// requires the signed-in user.
func (s *Store) CreateOAuthRequest(clientID, userID, redirectURI, state, codeChallenge, resource string, scopes []string) (*OAuthRequest, error) {
	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(userID) == "" ||
		strings.TrimSpace(redirectURI) == "" || strings.TrimSpace(codeChallenge) == "" ||
		strings.TrimSpace(resource) == "" || len(scopes) == 0 {
		return nil, ErrOAuthInvalidGrant
	}
	if _, err := s.GetOAuthClient(clientID); err != nil {
		return nil, err
	}
	id, err := randomToken(18)
	if err != nil {
		return nil, err
	}
	expires := time.Now().UTC().Add(10 * time.Minute)
	_, err = s.db.ExecContext(context.Background(),
		`INSERT INTO oauth_requests
		 (id, client_id, user_id, redirect_uri, state, code_challenge, resource, scopes, expires_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		id, clientID, userID, redirectURI, state, codeChallenge, resource,
		ScopeString(scopes), msFromTime(expires),
	)
	if err != nil {
		return nil, err
	}
	return &OAuthRequest{
		ID: id, ClientID: clientID, UserID: userID, RedirectURI: redirectURI,
		State: state, CodeChallenge: codeChallenge, Resource: resource,
		Scopes: scopes, ExpiresAt: expires,
	}, nil
}

// GetOAuthRequest returns a consent request belonging to userID.
func (s *Store) GetOAuthRequest(id, userID string) (*OAuthRequest, error) {
	var request OAuthRequest
	var scopes string
	var expiresMs int64
	err := s.db.QueryRowContext(context.Background(),
		`SELECT id, client_id, user_id, redirect_uri, state, code_challenge, resource, scopes, prepared_grant, expires_ms
		 FROM oauth_requests WHERE id = ? AND user_id = ? AND expires_ms > ?;`,
		id, userID, nowMs(),
	).Scan(&request.ID, &request.ClientID, &request.UserID, &request.RedirectURI,
		&request.State, &request.CodeChallenge, &request.Resource, &scopes, &request.PreparedGrant, &expiresMs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOAuthInvalidGrant
	}
	if err != nil {
		return nil, err
	}
	request.Scopes = strings.Fields(scopes)
	request.ExpiresAt = timeFromMs(expiresMs)
	return &request, nil
}

// TakeOAuthRequest atomically consumes a consent request belonging to userID.
func (s *Store) TakeOAuthRequest(id, userID string) (*OAuthRequest, error) {
	var request OAuthRequest
	var scopes string
	var expiresMs int64
	err := s.db.QueryRowContext(context.Background(),
		`DELETE FROM oauth_requests WHERE id = ? AND user_id = ? AND expires_ms > ?
		 RETURNING id, client_id, user_id, redirect_uri, state, code_challenge, resource, scopes, prepared_grant, expires_ms;`,
		id, userID, nowMs(),
	).Scan(&request.ID, &request.ClientID, &request.UserID, &request.RedirectURI,
		&request.State, &request.CodeChallenge, &request.Resource, &scopes, &request.PreparedGrant, &expiresMs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOAuthInvalidGrant
	}
	if err != nil {
		return nil, err
	}
	request.Scopes = strings.Fields(scopes)
	request.ExpiresAt = timeFromMs(expiresMs)
	return &request, nil
}

// DeleteOAuthRequest removes a pending consent request.
func (s *Store) DeleteOAuthRequest(id, userID string) error {
	_, err := s.db.ExecContext(context.Background(),
		`DELETE FROM oauth_requests WHERE id = ? AND user_id = ?;`, id, userID)
	return err
}

// SetOAuthRequestPreparedGrant stores the grant prepared for user consent.
func (s *Store) SetOAuthRequestPreparedGrant(id, userID, grant string) error {
	_, err := s.db.ExecContext(context.Background(),
		`UPDATE oauth_requests SET prepared_grant = ? WHERE id = ? AND user_id = ?;`,
		grant, id, userID)
	return err
}
