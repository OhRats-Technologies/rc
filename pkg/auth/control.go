package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ControlClient is a browser or CLI Ed25519 key authorized by a passkey.
type ControlClient struct {
	ID               string
	UserID           string
	Kind             string
	SigningPublicKey string
	CredentialID     string
	Grant            string
	Assertion        string
	CreatedAt        time.Time
	ExpiresAt        time.Time
}

// McpGrantRecord is the durable Owner-authorized MCP grant and its control proof.
type McpGrantRecord struct {
	ID               string
	UserID           string
	ClientID         string
	Name             string
	Grant            string
	Signature        string
	ControlClientID  string
	CredentialID     string
	ControlGrant     string
	ControlAssertion string
	CreatedAt        time.Time
	ExpiresAt        time.Time
	Revoked          bool
}

// PutControlClient stores or refreshes a control key authorization.
func (s *Store) PutControlClient(client ControlClient) error {
	result, err := s.db.ExecContext(context.Background(),
		`INSERT INTO control_clients
		 (id, user_id, kind, signing_public_key, credential_id, grant, assertion, created_ms, expires_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   kind=excluded.kind, signing_public_key=excluded.signing_public_key,
		   credential_id=excluded.credential_id, grant=excluded.grant,
		   assertion=excluded.assertion, created_ms=excluded.created_ms,
		   expires_ms=excluded.expires_ms
		 WHERE control_clients.user_id=excluded.user_id
		   AND control_clients.kind=excluded.kind;`,
		client.ID, client.UserID, client.Kind, client.SigningPublicKey,
		client.CredentialID, client.Grant, client.Assertion,
		msFromTime(client.CreatedAt), msFromTime(client.ExpiresAt),
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrControlClientConflict
	}
	return nil
}

// ControlClient returns an active control key belonging to userID.
func (s *Store) ControlClient(userID, id string) (*ControlClient, error) {
	var client ControlClient
	var createdMs, expiresMs int64
	err := s.db.QueryRowContext(context.Background(),
		`SELECT id, user_id, kind, signing_public_key, credential_id, grant, assertion, created_ms, expires_ms
		 FROM control_clients WHERE id = ? AND user_id = ? AND (expires_ms = 0 OR expires_ms > ?);`,
		id, userID, nowMs(),
	).Scan(&client.ID, &client.UserID, &client.Kind, &client.SigningPublicKey,
		&client.CredentialID, &client.Grant, &client.Assertion, &createdMs, &expiresMs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrControlClientNotFound
	}
	if err != nil {
		return nil, err
	}
	client.CreatedAt = timeFromMs(createdMs)
	client.ExpiresAt = timeFromMs(expiresMs)
	return &client, nil
}

// PutMcpGrant stores one signed MCP grant.
func (s *Store) PutMcpGrant(record McpGrantRecord) error {
	_, err := s.db.ExecContext(context.Background(),
		`INSERT INTO mcp_grants
		 (id, user_id, client_id, name, grant, signature, control_client_id,
		  credential_id, control_grant, control_assertion, created_ms, expires_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		record.ID, record.UserID, record.ClientID, record.Name, record.Grant,
		record.Signature, record.ControlClientID, record.CredentialID,
		record.ControlGrant, record.ControlAssertion,
		msFromTime(record.CreatedAt), msFromTime(record.ExpiresAt),
	)
	return err
}

// McpGrant returns one grant belonging to userID.
func (s *Store) McpGrant(userID, id string) (*McpGrantRecord, error) {
	var record McpGrantRecord
	var createdMs, expiresMs int64
	var revoked sql.NullInt64
	err := s.db.QueryRowContext(context.Background(),
		`SELECT id, user_id, client_id, name, grant, signature, control_client_id,
		        credential_id, control_grant, control_assertion, created_ms, expires_ms, revoked_ms
		 FROM mcp_grants WHERE id = ? AND user_id = ?;`,
		id, userID,
	).Scan(&record.ID, &record.UserID, &record.ClientID, &record.Name, &record.Grant,
		&record.Signature, &record.ControlClientID, &record.CredentialID,
		&record.ControlGrant, &record.ControlAssertion, &createdMs, &expiresMs, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOAuthInvalidGrant
	}
	if err != nil {
		return nil, err
	}
	record.CreatedAt = timeFromMs(createdMs)
	record.ExpiresAt = timeFromMs(expiresMs)
	record.Revoked = revoked.Valid
	if record.Revoked || (expiresMs != 0 && expiresMs <= nowMs()) {
		return nil, ErrOAuthInvalidGrant
	}
	return &record, nil
}

// RevokeMcpGrant revokes a grant and every OAuth token linked to it.
func (s *Store) RevokeMcpGrant(userID, id string) (bool, error) {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx,
		`UPDATE mcp_grants SET revoked_ms = ? WHERE id = ? AND user_id = ? AND revoked_ms IS NULL;`,
		nowMs(), id, userID)
	if err != nil {
		return false, err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE oauth_tokens SET revoked_ms = ? WHERE grant_id = ? AND revoked_ms IS NULL;`,
		nowMs(), id); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM authority_deliveries WHERE device_id IN (
		   SELECT id FROM devices WHERE workspace_id IN (
		     SELECT workspace_id FROM workspace_members WHERE user_id = ? AND role = 'owner'
		   )
		 );`, userID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// ActiveMcpGrants lists all unexpired, unrevoked grants for authority snapshots.
func (s *Store) ActiveMcpGrants() ([]McpGrantRecord, error) {
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT id, user_id, client_id, name, grant, signature, control_client_id,
		        credential_id, control_grant, control_assertion, created_ms, expires_ms
		 FROM mcp_grants WHERE revoked_ms IS NULL AND (expires_ms = 0 OR expires_ms > ?)
		 ORDER BY id;`, nowMs())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []McpGrantRecord
	for rows.Next() {
		var record McpGrantRecord
		var createdMs, expiresMs int64
		if err := rows.Scan(&record.ID, &record.UserID, &record.ClientID, &record.Name,
			&record.Grant, &record.Signature, &record.ControlClientID, &record.CredentialID,
			&record.ControlGrant, &record.ControlAssertion, &createdMs, &expiresMs); err != nil {
			return nil, err
		}
		record.CreatedAt = timeFromMs(createdMs)
		record.ExpiresAt = timeFromMs(expiresMs)
		records = append(records, record)
	}
	return records, rows.Err()
}

// McpGrantsForUser lists active grants belonging to one account.
func (s *Store) McpGrantsForUser(userID string) ([]McpGrantRecord, error) {
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT id, user_id, client_id, name, grant, signature, control_client_id,
		        credential_id, control_grant, control_assertion, created_ms, expires_ms
		 FROM mcp_grants WHERE user_id = ? AND revoked_ms IS NULL AND (expires_ms = 0 OR expires_ms > ?)
		 ORDER BY created_ms DESC, id;`, userID, nowMs())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []McpGrantRecord
	for rows.Next() {
		var record McpGrantRecord
		var createdMs, expiresMs int64
		if err := rows.Scan(&record.ID, &record.UserID, &record.ClientID, &record.Name,
			&record.Grant, &record.Signature, &record.ControlClientID, &record.CredentialID,
			&record.ControlGrant, &record.ControlAssertion, &createdMs, &expiresMs); err != nil {
			return nil, err
		}
		record.CreatedAt = timeFromMs(createdMs)
		record.ExpiresAt = timeFromMs(expiresMs)
		records = append(records, record)
	}
	return records, rows.Err()
}
