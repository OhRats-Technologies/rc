package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"
)

const maxOAuthRedirectURIs = 10

// RegisterOAuthClient creates an MCP OAuth client from dynamic client
// registration metadata. Redirect URIs are matched exactly later.
func (s *Store) RegisterOAuthClient(name string, redirectURIs []string) (string, error) {
	displayName := strings.TrimSpace(name)
	if displayName == "" {
		displayName = "MCP client"
	}
	if len(displayName) > maxNameBytes || !utf8.ValidString(displayName) {
		return "", ErrInvalidName
	}
	normalized, err := normalizeRedirectURIs(redirectURIs)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	clientID, err := randomToken(16)
	if err != nil {
		return "", err
	}
	_, err = s.db.ExecContext(
		context.Background(),
		`INSERT INTO oauth_clients (client_id, client_name, redirect_uris, created_ms)
		 VALUES (?, ?, ?, ?);`,
		clientID,
		displayName,
		string(encoded),
		nowMs(),
	)
	if err != nil {
		return "", err
	}
	return clientID, nil
}

// GetOAuthClient returns one registered client.
func (s *Store) GetOAuthClient(id string) (*OAuthClient, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrOAuthClientNotFound
	}
	var name, redirectURIs string
	var createdMs int64
	err := s.db.QueryRowContext(
		context.Background(),
		`SELECT client_name, redirect_uris, created_ms FROM oauth_clients WHERE client_id = ?;`,
		id,
	).Scan(&name, &redirectURIs, &createdMs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOAuthClientNotFound
	}
	if err != nil {
		return nil, err
	}
	var uris []string
	if err := json.Unmarshal([]byte(redirectURIs), &uris); err != nil {
		return nil, err
	}
	return &OAuthClient{
		ID:           id,
		Name:         name,
		RedirectURIs: uris,
		CreatedAt:    timeFromMs(createdMs),
	}, nil
}

// ValidateRedirectURI reports whether uri is registered exactly for clientID.
func (s *Store) ValidateRedirectURI(clientID, uri string) error {
	client, err := s.GetOAuthClient(clientID)
	if err != nil {
		return err
	}
	for _, candidate := range client.RedirectURIs {
		if candidate == uri {
			return nil
		}
	}
	return ErrOAuthInvalidRedirect
}

func normalizeRedirectURIs(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > maxOAuthRedirectURIs {
		return nil, ErrOAuthInvalidRedirect
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if err := validateRedirectURI(value); err != nil {
			return nil, err
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func validateRedirectURI(value string) error {
	if value == "" || strings.TrimSpace(value) != value || strings.Contains(value, "#") {
		return ErrOAuthInvalidRedirect
	}
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.User != nil {
		return ErrOAuthInvalidRedirect
	}
	if parsed.Fragment != "" || parsed.RawFragment != "" {
		return ErrOAuthInvalidRedirect
	}
	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		switch parsed.Hostname() {
		case "localhost", "127.0.0.1", "::1":
			return nil
		}
	}
	return ErrOAuthInvalidRedirect
}
