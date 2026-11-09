package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// AuthorityDelivery is one signed RC Lock transition queued for a Node.
type AuthorityDelivery struct {
	DeviceID     string
	PreviousHash string
	Generation   uint64
	NextHash     string
	Message      string
	ExpiresAt    time.Time
}

// PutAuthorityDelivery stores one signed transition for a Node.
func (s *Store) PutAuthorityDelivery(delivery AuthorityDelivery) error {
	_, err := s.db.ExecContext(context.Background(),
		`INSERT INTO authority_deliveries
		 (device_id, previous_hash, generation, next_hash, message, expires_ms)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(device_id, previous_hash, generation) DO UPDATE SET
		   next_hash=excluded.next_hash, message=excluded.message, expires_ms=excluded.expires_ms;`,
		delivery.DeviceID, delivery.PreviousHash, delivery.Generation,
		delivery.NextHash, delivery.Message, msFromTime(delivery.ExpiresAt),
	)
	return err
}

// AuthorityDelivery returns the next queued transition matching a Node's state.
func (s *Store) AuthorityDelivery(deviceID, hash string, generation uint64) (*AuthorityDelivery, error) {
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM authority_deliveries WHERE expires_ms <> 0 AND expires_ms <= ?;`,
		nowMs()); err != nil {
		return nil, err
	}
	var delivery AuthorityDelivery
	var expiresMs int64
	err := s.db.QueryRowContext(ctx,
		`SELECT device_id, previous_hash, generation, next_hash, message, expires_ms
		 FROM authority_deliveries
		 WHERE device_id = ? AND previous_hash = ? AND generation = ?
		   AND (expires_ms = 0 OR expires_ms > ?);`,
		deviceID, hash, generation, nowMs(),
	).Scan(&delivery.DeviceID, &delivery.PreviousHash, &delivery.Generation,
		&delivery.NextHash, &delivery.Message, &expiresMs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	delivery.ExpiresAt = timeFromMs(expiresMs)
	return &delivery, nil
}

// AuthorityDeliveryParents lists edges that need to be signed for a workspace.
func (s *Store) AuthorityDeliveryParents(workspaceID string) ([][2]any, error) {
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT DISTINCT q.next_hash, q.generation + 1 FROM authority_deliveries q
		 JOIN devices d ON d.id = q.device_id WHERE d.workspace_id = ?
		 AND (q.expires_ms = 0 OR q.expires_ms > ?) ORDER BY q.next_hash;`,
		workspaceID, nowMs())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result [][2]any
	for rows.Next() {
		var hash string
		var generation int64
		if err := rows.Scan(&hash, &generation); err != nil {
			return nil, err
		}
		result = append(result, [2]any{hash, uint64(generation)})
	}
	return result, rows.Err()
}

// DeleteAuthorityDeliveries removes transitions the Node has already passed.
func (s *Store) DeleteAuthorityDeliveries(deviceID, hash string, generation uint64) error {
	_, err := s.db.ExecContext(context.Background(),
		`DELETE FROM authority_deliveries WHERE device_id = ?
		 AND (generation < ? OR (generation = ? AND previous_hash <> ?));`,
		deviceID, generation, generation, hash)
	return err
}

// UpdateDeviceLockState records the Node-acknowledged RC Lock state.
func (s *Store) UpdateDeviceLockState(deviceID, hash string, generation uint64) error {
	_, err := s.db.ExecContext(context.Background(),
		`UPDATE devices SET lock_hash = ?, lock_generation = ? WHERE id = ?;`,
		hash, generation, deviceID)
	return err
}

// EnsureWorkspaceMembership adds a user to a workspace when absent.
func (s *Store) EnsureWorkspaceMembership(workspaceID, userID, role string) error {
	if role != "owner" && role != "operator" && role != "viewer" {
		return ErrInvalidName
	}
	_, err := s.db.ExecContext(context.Background(),
		`INSERT OR IGNORE INTO workspace_members (workspace_id, user_id, role, joined_ms)
		 VALUES (?, ?, ?, ?);`, workspaceID, userID, role, nowMs())
	return err
}

// SetWorkspaceMembership creates or updates a user's role in a workspace.
func (s *Store) SetWorkspaceMembership(workspaceID, userID, role string) error {
	if role != "owner" && role != "operator" && role != "viewer" {
		return ErrInvalidName
	}
	_, err := s.db.ExecContext(context.Background(),
		`INSERT INTO workspace_members (workspace_id, user_id, role, joined_ms)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(workspace_id, user_id) DO UPDATE SET role = excluded.role;`,
		workspaceID, userID, role, nowMs())
	return err
}

// WorkspaceRole returns a user's role in a workspace.
func (s *Store) WorkspaceRole(userID, workspaceID string) (string, error) {
	var role string
	err := s.db.QueryRowContext(context.Background(),
		`SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?;`,
		workspaceID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrUserNotFound
	}
	return role, err
}

// WorkspaceMembers lists member IDs and roles.
func (s *Store) WorkspaceMembers(workspaceID string) ([]Membership, error) {
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT workspace_id, user_id, role, joined_ms FROM workspace_members
		 WHERE workspace_id = ? ORDER BY user_id;`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []Membership
	for rows.Next() {
		var member Membership
		var joinedMs int64
		if err := rows.Scan(&member.WorkspaceID, &member.UserID, &member.Role, &joinedMs); err != nil {
			return nil, err
		}
		member.JoinedAt = timeFromMs(joinedMs)
		members = append(members, member)
	}
	return members, rows.Err()
}

// ListUsers returns every account.
func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT id, name, created_ms FROM users ORDER BY id;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var user User
		var createdMs int64
		if err := rows.Scan(&user.ID, &user.Name, &createdMs); err != nil {
			return nil, err
		}
		user.CreatedAt = timeFromMs(createdMs)
		users = append(users, user)
	}
	return users, rows.Err()
}

// WorkspaceAccess is one workspace visible to an account.
type WorkspaceAccess struct {
	ID   string
	Name string
	Role string
}

// WorkspacesForUser lists every workspace membership for an account.
func (s *Store) WorkspacesForUser(userID string) ([]WorkspaceAccess, error) {
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT w.id, w.name, wm.role FROM workspace_members wm
		 JOIN workspaces w ON w.id = wm.workspace_id
		 WHERE wm.user_id = ? ORDER BY w.created_ms, w.id;`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var workspaces []WorkspaceAccess
	for rows.Next() {
		var workspace WorkspaceAccess
		if err := rows.Scan(&workspace.ID, &workspace.Name, &workspace.Role); err != nil {
			return nil, err
		}
		workspaces = append(workspaces, workspace)
	}
	return workspaces, rows.Err()
}

// WorkspacesForOwner lists workspaces where userID is an Owner.
func (s *Store) WorkspacesForOwner(userID string) ([]string, error) {
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT workspace_id FROM workspace_members WHERE user_id = ? AND role = 'owner' ORDER BY workspace_id;`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// AuthorityDeliveryParentsForDevice lists pending edges for one Node.
func (s *Store) AuthorityDeliveryParentsForDevice(deviceID string) ([][2]any, error) {
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT next_hash, generation + 1 FROM authority_deliveries
		 WHERE device_id = ? AND (expires_ms = 0 OR expires_ms > ?) ORDER BY next_hash;`,
		deviceID, nowMs())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result [][2]any
	for rows.Next() {
		var hash string
		var generation int64
		if err := rows.Scan(&hash, &generation); err != nil {
			return nil, err
		}
		result = append(result, [2]any{hash, uint64(generation)})
	}
	return result, rows.Err()
}
