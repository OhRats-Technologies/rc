package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"time"
)

// MaxPendingWorkspaceInvites bounds unconsumed invites per workspace.
const MaxPendingWorkspaceInvites = 25

// WorkspaceInvite is one pending owner-created workspace invitation.
type WorkspaceInvite struct {
	ID          string
	WorkspaceID string
	Role        string
	CreatedBy   string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

func inviteTokenHash(token string) []byte {
	digest := sha256.Sum256([]byte(token))
	return digest[:]
}

// CreateWorkspaceInvite stores a hashed invite token and returns the plaintext
// token exactly once.
func (s *Store) CreateWorkspaceInvite(workspaceID, createdBy, role string, expires time.Time) (string, string, error) {
	if role != "owner" && role != "operator" && role != "viewer" {
		return "", "", ErrInvalidName
	}
	var pending int
	if err := s.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM workspace_invites WHERE workspace_id = ? AND used_ms IS NULL AND expires_ms > ?;`,
		workspaceID, nowMs()).Scan(&pending); err != nil {
		return "", "", err
	}
	if pending >= MaxPendingWorkspaceInvites {
		return "", "", ErrWorkspaceInviteLimit
	}
	token, err := randomToken(24)
	if err != nil {
		return "", "", err
	}
	token = "invite_" + token
	id, err := randomToken(16)
	if err != nil {
		return "", "", err
	}
	id = "inv_" + id
	_, err = s.db.ExecContext(context.Background(),
		`INSERT INTO workspace_invites
		 (id, workspace_id, token_hash, role, created_by, created_ms, expires_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?);`,
		id, workspaceID, inviteTokenHash(token), role, createdBy, nowMs(), msFromTime(expires))
	if err != nil {
		return "", "", err
	}
	return id, token, nil
}

// ListWorkspaceInvites lists active pending invites.
func (s *Store) ListWorkspaceInvites(workspaceID string) ([]WorkspaceInvite, error) {
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT id, workspace_id, role, created_by, created_ms, expires_ms
		 FROM workspace_invites WHERE workspace_id = ? AND used_ms IS NULL AND expires_ms > ?
		 ORDER BY created_ms DESC, id;`, workspaceID, nowMs())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	invites := make([]WorkspaceInvite, 0)
	for rows.Next() {
		var invite WorkspaceInvite
		var createdMs, expiresMs int64
		if err := rows.Scan(&invite.ID, &invite.WorkspaceID, &invite.Role,
			&invite.CreatedBy, &createdMs, &expiresMs); err != nil {
			return nil, err
		}
		invite.CreatedAt = timeFromMs(createdMs)
		invite.ExpiresAt = timeFromMs(expiresMs)
		invites = append(invites, invite)
	}
	return invites, rows.Err()
}

// RevokeWorkspaceInvite removes one unused invite.
func (s *Store) RevokeWorkspaceInvite(workspaceID, id string) (bool, error) {
	result, err := s.db.ExecContext(context.Background(),
		`DELETE FROM workspace_invites WHERE id = ? AND workspace_id = ? AND used_ms IS NULL;`,
		id, workspaceID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

// WorkspaceInviteValid reports whether an invite token is still usable.
func (s *Store) WorkspaceInviteValid(token string) bool {
	var count int
	err := s.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM workspace_invites
		 WHERE token_hash = ? AND used_ms IS NULL AND expires_ms > ?;`,
		inviteTokenHash(token), nowMs()).Scan(&count)
	return err == nil && count == 1
}

// ConsumeWorkspaceInvite atomically marks an invite used and adds the account.
func (s *Store) ConsumeWorkspaceInvite(token, userID string) (string, string, error) {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = tx.Rollback() }()
	var inviteID, workspaceID, role string
	err = tx.QueryRowContext(ctx,
		`SELECT id, workspace_id, role FROM workspace_invites
		 WHERE token_hash = ? AND used_ms IS NULL AND expires_ms > ?;`,
		inviteTokenHash(token), nowMs()).Scan(&inviteID, &workspaceID, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrWorkspaceInviteNotFound
	}
	if err != nil {
		return "", "", err
	}
	result, err := tx.ExecContext(ctx,
		`UPDATE workspace_invites SET used_ms = ? WHERE id = ? AND used_ms IS NULL;`,
		nowMs(), inviteID)
	if err != nil {
		return "", "", err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return "", "", err
	}
	if affected != 1 {
		return "", "", ErrWorkspaceInviteNotFound
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO workspace_members (workspace_id, user_id, role, joined_ms)
		 VALUES (?, ?, ?, ?);`, workspaceID, userID, role, nowMs()); err != nil {
		return "", "", err
	}
	if err := tx.Commit(); err != nil {
		return "", "", err
	}
	return workspaceID, role, nil
}
