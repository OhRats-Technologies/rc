package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ScheduleGrant is an Owner-authorized immutable schedule permit projected into
// RC Lock. It contains no command or environment content.
type ScheduleGrant struct {
	ScheduleID   string
	WorkspaceID  string
	DeviceID     string
	UserID       string
	SpecHash     string
	MaxRuntimeMs uint64
	ExpiresAt    time.Time
	CreatedAt    time.Time
}

// PutScheduleGrant creates or replaces a permit for one workspace/device.
func (s *Store) PutScheduleGrant(grant ScheduleGrant) error {
	ctx := context.Background()
	var deviceCount int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM devices WHERE id = ? AND workspace_id = ?;`,
		grant.DeviceID, grant.WorkspaceID).Scan(&deviceCount); err != nil {
		return err
	}
	if deviceCount == 0 {
		return ErrScheduleGrantNotFound
	}
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO schedule_grants
		 (schedule_id, workspace_id, device_id, user_id, spec_hash,
		  max_runtime_ms, expires_ms, created_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(schedule_id) DO UPDATE SET
		   device_id=excluded.device_id, user_id=excluded.user_id,
		   spec_hash=excluded.spec_hash, max_runtime_ms=excluded.max_runtime_ms,
		   expires_ms=excluded.expires_ms, created_ms=excluded.created_ms
		 WHERE schedule_grants.workspace_id=excluded.workspace_id;`,
		grant.ScheduleID, grant.WorkspaceID, grant.DeviceID, grant.UserID,
		grant.SpecHash, grant.MaxRuntimeMs, msFromTime(grant.ExpiresAt),
		msFromTime(grant.CreatedAt),
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrScheduleGrantNotFound
	}
	return nil
}

// ScheduleGrant returns one permit scoped to its workspace.
func (s *Store) ScheduleGrant(workspaceID, scheduleID string) (*ScheduleGrant, error) {
	row := s.db.QueryRowContext(context.Background(),
		`SELECT schedule_id, workspace_id, device_id, user_id, spec_hash,
		        max_runtime_ms, expires_ms, created_ms
		 FROM schedule_grants WHERE workspace_id = ? AND schedule_id = ?;`,
		workspaceID, scheduleID)
	grant, err := scanScheduleGrant(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrScheduleGrantNotFound
	}
	return grant, err
}

// DeleteScheduleGrant removes one workspace-scoped permit.
func (s *Store) DeleteScheduleGrant(workspaceID, scheduleID string) (bool, error) {
	result, err := s.db.ExecContext(context.Background(),
		`DELETE FROM schedule_grants WHERE workspace_id = ? AND schedule_id = ?;`,
		workspaceID, scheduleID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

// ActiveScheduleGrantsForWorkspace lists unexpired permits canonically.
func (s *Store) ActiveScheduleGrantsForWorkspace(workspaceID string) ([]ScheduleGrant, error) {
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT schedule_id, workspace_id, device_id, user_id, spec_hash,
		        max_runtime_ms, expires_ms, created_ms
		 FROM schedule_grants
		 WHERE workspace_id = ? AND (expires_ms = 0 OR expires_ms > ?)
		 ORDER BY schedule_id;`, workspaceID, nowMs())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var grants []ScheduleGrant
	for rows.Next() {
		grant, err := scanScheduleGrant(rows)
		if err != nil {
			return nil, err
		}
		grants = append(grants, *grant)
	}
	return grants, rows.Err()
}

type scheduleGrantScanner interface {
	Scan(dest ...any) error
}

func scanScheduleGrant(row scheduleGrantScanner) (*ScheduleGrant, error) {
	var grant ScheduleGrant
	var maxRuntime, expiresMs, createdMs int64
	if err := row.Scan(&grant.ScheduleID, &grant.WorkspaceID, &grant.DeviceID,
		&grant.UserID, &grant.SpecHash, &maxRuntime, &expiresMs, &createdMs); err != nil {
		return nil, err
	}
	grant.MaxRuntimeMs = uint64(maxRuntime)
	if expiresMs != 0 {
		grant.ExpiresAt = timeFromMs(expiresMs)
	}
	grant.CreatedAt = timeFromMs(createdMs)
	return &grant, nil
}
