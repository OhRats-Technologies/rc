package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const MaxWorkspacesPerCreator = 10

// CreateWorkspace creates a durable workspace and makes the creator its first
// Owner.
func (s *Store) CreateWorkspace(name, createdBy string) (*Workspace, error) {
	name, err := normalizeName(name)
	if err != nil {
		return nil, err
	}
	if createdBy == "" {
		return nil, ErrUserNotFound
	}
	var count int
	if err := s.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM workspaces WHERE created_by = ?;`, createdBy).Scan(&count); err != nil {
		return nil, err
	}
	if count >= MaxWorkspacesPerCreator {
		return nil, ErrWorkspaceLimit
	}
	token, err := randomToken(16)
	if err != nil {
		return nil, err
	}
	workspace := Workspace{ID: "ws_" + token, Name: name, CreatedBy: createdBy, CreatedAt: time.Now().UTC()}
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(context.Background(),
		`INSERT INTO workspaces (id, name, created_by, created_ms) VALUES (?, ?, ?, ?);`,
		workspace.ID, workspace.Name, createdBy, msFromTime(workspace.CreatedAt)); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(context.Background(),
		`INSERT INTO workspace_members (workspace_id, user_id, role, joined_ms) VALUES (?, ?, 'owner', ?);`,
		workspace.ID, createdBy, msFromTime(workspace.CreatedAt)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &workspace, nil
}

// RenameWorkspace changes a workspace display name.
func (s *Store) RenameWorkspace(id, name string) error {
	name, err := normalizeName(name)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(context.Background(),
		`UPDATE workspaces SET name = ? WHERE id = ?;`, name, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrWorkspaceNotFound
	}
	return nil
}

// DeleteWorkspace removes a workspace and its workspace-scoped rows.
func (s *Store) DeleteWorkspace(id string) error {
	return s.db.WithTx(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(context.Background(),
			`DELETE FROM authority_deliveries WHERE device_id IN (SELECT id FROM devices WHERE workspace_id = ?);`,
			id); err != nil {
			return err
		}
		result, err := tx.ExecContext(context.Background(), `DELETE FROM workspaces WHERE id = ?;`, id)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrWorkspaceNotFound
		}
		return nil
	})
}

// LeaveWorkspace removes one membership while preserving another Owner.
func (s *Store) LeaveWorkspace(workspaceID, userID string) (string, error) {
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	role, err := workspaceMemberRole(tx, workspaceID, userID)
	if errors.Is(err, ErrWorkspaceMemberNotFound) {
		return "", ErrWorkspaceNotFound
	}
	if err != nil {
		return "", err
	}
	if role == "owner" {
		if err := requireAnotherOwner(tx, workspaceID); err != nil {
			return "", err
		}
	}
	if _, err := tx.ExecContext(context.Background(),
		`DELETE FROM workspace_members WHERE workspace_id = ? AND user_id = ?;`,
		workspaceID, userID); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return role, nil
}
