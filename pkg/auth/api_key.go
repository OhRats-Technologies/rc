package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// APIKeyScopes are the supported proof-of-possession API scopes in canonical
// order. Unknown values are ignored.
var APIKeyScopes = []string{"read", "execute", "manage-devices", "manage-workspaces"}

// MaxAPIKeysPerUser bounds durable API signing keys per account.
const MaxAPIKeysPerUser = 10

// APIKey is an Ed25519 proof-of-possession credential. Only the public key is
// stored.
type APIKey struct {
	ID         string
	UserID     string
	Name       string
	PublicKey  string
	Scopes     []string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastUsedAt *time.Time
}

// Active reports whether the key is still usable at now.
func (key *APIKey) Active(now time.Time) bool {
	return key.PublicKey != "" && (key.ExpiresAt.IsZero() || key.ExpiresAt.After(now))
}

// NormalizeAPIKeyScopes filters unknown scopes, preserves canonical order, and
// defaults to read + execute when no supported scope is selected.
func NormalizeAPIKeyScopes(values []string) []string {
	selected := make(map[string]bool, len(values))
	for _, value := range values {
		selected[value] = true
	}
	scopes := make([]string, 0, len(APIKeyScopes))
	for _, scope := range APIKeyScopes {
		if selected[scope] {
			scopes = append(scopes, scope)
		}
	}
	if len(scopes) == 0 {
		scopes = []string{"read", "execute"}
	}
	return scopes
}

// PutAPIKey stores a new public signing key.
func (s *Store) PutAPIKey(key APIKey) error {
	if key.ID == "" || key.UserID == "" || key.PublicKey == "" {
		return ErrInvalidCredential
	}
	scopes := NormalizeAPIKeyScopes(key.Scopes)
	encoded, err := json.Marshal(scopes)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(context.Background(),
		`INSERT INTO api_keys
		 (id, user_id, name, public_key, scopes, created_ms, expires_ms, last_used_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?);`,
		key.ID, key.UserID, key.Name, key.PublicKey, string(encoded),
		msFromTime(key.CreatedAt), msFromTime(key.ExpiresAt), nullableMs(key.LastUsedAt),
	)
	return err
}

const apiKeySelect = `SELECT api_keys.id, api_keys.user_id, api_keys.name, api_keys.public_key,
	api_keys.scopes, api_keys.created_ms, api_keys.expires_ms, api_keys.last_used_ms
	FROM api_keys`

// APIKey returns one key regardless of expiry so callers can distinguish an
// expired key from an unknown one when auditing.
func (s *Store) APIKey(id string) (*APIKey, error) {
	key, err := scanAPIKey(s.db.QueryRowContext(context.Background(), apiKeySelect+` WHERE id = ?;`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAPIKeyNotFound
	}
	if err != nil {
		return nil, err
	}
	return key, nil
}

// ListAPIKeys lists an account's keys, newest first.
func (s *Store) ListAPIKeys(userID string) ([]APIKey, error) {
	rows, err := s.db.QueryContext(context.Background(),
		apiKeySelect+` WHERE user_id = ? ORDER BY created_ms DESC, id;`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []APIKey
	for rows.Next() {
		key, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, *key)
	}
	return keys, rows.Err()
}

// CountAPIKeys reports how many durable keys an account owns.
func (s *Store) CountAPIKeys(userID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM api_keys WHERE user_id = ?;`, userID).Scan(&count)
	return count, err
}

// DeleteAPIKey removes a key owned by userID.
func (s *Store) DeleteAPIKey(userID, id string) (bool, error) {
	result, err := s.db.ExecContext(context.Background(),
		`DELETE FROM api_keys WHERE id = ? AND user_id = ?;`, id, userID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

// ActiveAPIKeysForWorkspace lists active member keys canonically by ID for RC
// Lock projection.
func (s *Store) ActiveAPIKeysForWorkspace(workspaceID string) ([]APIKey, error) {
	rows, err := s.db.QueryContext(context.Background(),
		apiKeySelect+` JOIN workspace_members wm ON wm.user_id = api_keys.user_id
		WHERE wm.workspace_id = ? AND api_keys.public_key <> ''
		  AND (api_keys.expires_ms = 0 OR api_keys.expires_ms > ?)
		ORDER BY api_keys.id;`, workspaceID, nowMs())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []APIKey
	for rows.Next() {
		key, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, *key)
	}
	return keys, rows.Err()
}

// TouchAPIKey records successful credential use.
func (s *Store) TouchAPIKey(id string) error {
	_, err := s.db.ExecContext(context.Background(),
		`UPDATE api_keys SET last_used_ms = ? WHERE id = ?;`, nowMs(), id)
	return err
}

// RememberNonce atomically records a request nonce. It returns false when the
// nonce was already accepted within its retention window.
func (s *Store) RememberNonce(principal, nonce string, expires time.Time) (bool, error) {
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM request_nonces WHERE expires_ms <= ?;`, nowMs()); err != nil {
		return false, err
	}
	hash := sha256.Sum256([]byte(nonce))
	result, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO request_nonces (principal, nonce_hash, expires_ms) VALUES (?, ?, ?);`,
		principal, hash[:], msFromTime(expires))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

type apiKeyScanner interface {
	Scan(dest ...any) error
}

func scanAPIKey(row apiKeyScanner) (*APIKey, error) {
	var key APIKey
	var scopes string
	var createdMs, expiresMs int64
	var lastUsed sql.NullInt64
	if err := row.Scan(&key.ID, &key.UserID, &key.Name, &key.PublicKey,
		&scopes, &createdMs, &expiresMs, &lastUsed); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(scopes), &key.Scopes); err != nil {
		return nil, err
	}
	key.Scopes = NormalizeAPIKeyScopes(key.Scopes)
	key.CreatedAt = timeFromMs(createdMs)
	if expiresMs != 0 {
		key.ExpiresAt = timeFromMs(expiresMs)
	}
	if lastUsed.Valid {
		value := timeFromMs(lastUsed.Int64)
		key.LastUsedAt = &value
	}
	return &key, nil
}

// NormalizeAPIKeyName trims a display name and applies the canonical fallback.
func NormalizeAPIKeyName(value string) string {
	name := strings.TrimSpace(value)
	if name == "" {
		return "API key"
	}
	runes := []rune(name)
	if len(runes) > 80 {
		name = string(runes[:80])
	}
	return name
}
