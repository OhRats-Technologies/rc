package auth

import (
	"context"
	"encoding/json"
	"time"
)

// AuditEvent is bounded workspace metadata. It never contains process input,
// output, credentials, or command text.
type AuditEvent struct {
	ID          int64
	WorkspaceID string
	UserID      string
	Kind        string
	Detail      string
	DeviceID    string
	CreatedAt   time.Time
}

// RecordAuditEvent stores one bounded workspace audit event.
func (s *Store) RecordAuditEvent(event AuditEvent) error {
	if event.WorkspaceID == "" || event.Kind == "" {
		return ErrInvalidName
	}
	detail := event.Detail
	if detail == "" {
		detail = "{}"
	}
	var valid any
	if err := json.Unmarshal([]byte(detail), &valid); err != nil {
		return ErrInvalidName
	}
	_, err := s.db.ExecContext(context.Background(),
		`INSERT INTO audit_events (workspace_id, user_id, kind, detail, device_id, created_ms)
		 VALUES (?, NULLIF(?, ''), ?, ?, NULLIF(?, ''), ?);`,
		event.WorkspaceID, event.UserID, event.Kind, detail, event.DeviceID, nowMs())
	return err
}

// WorkspaceAuditEvents returns the newest bounded event window for a workspace.
func (s *Store) WorkspaceAuditEvents(workspaceID string, limit int) ([]AuditEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT id, workspace_id, COALESCE(user_id, ''), kind, detail,
		        COALESCE(device_id, ''), created_ms
		 FROM audit_events WHERE workspace_id = ? ORDER BY created_ms DESC, id DESC LIMIT ?;`,
		workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]AuditEvent, 0)
	for rows.Next() {
		var event AuditEvent
		var createdMs int64
		if err := rows.Scan(&event.ID, &event.WorkspaceID, &event.UserID, &event.Kind,
			&event.Detail, &event.DeviceID, &createdMs); err != nil {
			return nil, err
		}
		event.CreatedAt = timeFromMs(createdMs)
		events = append(events, event)
	}
	return events, rows.Err()
}
