package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// WorkspaceMemberDetail is one workspace member with its account name.
type WorkspaceMemberDetail struct {
	UserID   string
	Name     string
	Role     string
	JoinedAt time.Time
}

// WorkspaceByID returns one durable workspace.
func (s *Store) WorkspaceByID(id string) (*Workspace, error) {
	var workspace Workspace
	var createdBy sql.NullString
	var createdMs int64
	err := s.db.QueryRowContext(context.Background(),
		`SELECT id, name, created_by, created_ms FROM workspaces WHERE id = ?;`, id).
		Scan(&workspace.ID, &workspace.Name, &createdBy, &createdMs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrWorkspaceNotFound
	}
	if err != nil {
		return nil, err
	}
	if createdBy.Valid {
		workspace.CreatedBy = createdBy.String
	}
	workspace.CreatedAt = timeFromMs(createdMs)
	return &workspace, nil
}

// WorkspaceMemberDetails lists members with their account names.
func (s *Store) WorkspaceMemberDetails(workspaceID string) ([]WorkspaceMemberDetail, error) {
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT wm.user_id, u.name, wm.role, wm.joined_ms
		 FROM workspace_members wm JOIN users u ON u.id = wm.user_id
		 WHERE wm.workspace_id = ? ORDER BY wm.joined_ms, wm.user_id;`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := make([]WorkspaceMemberDetail, 0)
	for rows.Next() {
		var member WorkspaceMemberDetail
		var joinedMs int64
		if err := rows.Scan(&member.UserID, &member.Name, &member.Role, &joinedMs); err != nil {
			return nil, err
		}
		member.JoinedAt = timeFromMs(joinedMs)
		members = append(members, member)
	}
	return members, rows.Err()
}

// UpdateWorkspaceMemberRole changes one role atomically and refuses to remove
// the last owner.
func (s *Store) UpdateWorkspaceMemberRole(workspaceID, userID, role string) (string, error) {
	if role != "owner" && role != "operator" && role != "viewer" {
		return "", ErrInvalidName
	}
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := workspaceMemberRole(tx, workspaceID, userID)
	if err != nil {
		return "", err
	}
	if current == "owner" && role != "owner" {
		if err := requireAnotherOwner(tx, workspaceID); err != nil {
			return "", err
		}
	}
	if _, err := tx.ExecContext(context.Background(),
		`UPDATE workspace_members SET role = ? WHERE workspace_id = ? AND user_id = ?;`,
		role, workspaceID, userID); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return current, nil
}

// RemoveWorkspaceMember removes one member and refuses to remove the last owner.
func (s *Store) RemoveWorkspaceMember(workspaceID, userID string) (string, error) {
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := workspaceMemberRole(tx, workspaceID, userID)
	if err != nil {
		return "", err
	}
	if current == "owner" {
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
	return current, nil
}

func workspaceMemberRole(tx *sql.Tx, workspaceID, userID string) (string, error) {
	var role string
	err := tx.QueryRowContext(context.Background(),
		`SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?;`,
		workspaceID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrWorkspaceMemberNotFound
	}
	return role, err
}

func requireAnotherOwner(tx *sql.Tx, workspaceID string) error {
	var owners int
	if err := tx.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM workspace_members WHERE workspace_id = ? AND role = 'owner';`,
		workspaceID).Scan(&owners); err != nil {
		return err
	}
	if owners <= 1 {
		return ErrLastWorkspaceOwner
	}
	return nil
}
