package auth

import (
	"context"
	"database/sql"
)

// DeleteAccount removes an account, deletes every workspace it owns, revokes
// those workspaces' devices, and preserves surviving historical attribution as
// the Deleted account principal. It returns the revoked device IDs so the
// server can evict its in-memory registry and close live Node transports.
func (s *Store) DeleteAccount(userID string) ([]string, error) {
	if userID == "" || userID == DeletedAccountID {
		return nil, ErrUserNotFound
	}
	var devices []string
	err := s.db.WithTx(context.Background(), func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM users WHERE id = ?;`, userID).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return ErrUserNotFound
		}
		if _, err := tx.ExecContext(context.Background(),
			`INSERT OR IGNORE INTO users (id, name, created_ms) VALUES (?, ?, ?);`,
			DeletedAccountID, DeletedAccountName, nowMs()); err != nil {
			return err
		}
		rows, err := tx.QueryContext(context.Background(),
			`SELECT workspace_id FROM workspace_members WHERE user_id = ? AND role = 'owner' ORDER BY workspace_id;`,
			userID)
		if err != nil {
			return err
		}
		var owned []string
		for rows.Next() {
			var workspaceID string
			if err := rows.Scan(&workspaceID); err != nil {
				rows.Close()
				return err
			}
			owned = append(owned, workspaceID)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, workspaceID := range owned {
			deviceIDs, err := revokeWorkspaceDevices(tx, workspaceID)
			if err != nil {
				return err
			}
			devices = append(devices, deviceIDs...)
			if _, err := tx.ExecContext(context.Background(),
				`DELETE FROM workspaces WHERE id = ?;`, workspaceID); err != nil {
				return err
			}
		}
		for _, statement := range []string{
			`UPDATE workspaces SET created_by = ? WHERE created_by = ?;`,
			`UPDATE workspace_invites SET created_by = ? WHERE created_by = ?;`,
			`UPDATE audit_events SET user_id = ? WHERE user_id = ?;`,
		} {
			if _, err := tx.ExecContext(context.Background(), statement, DeletedAccountID, userID); err != nil {
				return err
			}
		}
		for _, statement := range []string{
			`DELETE FROM oauth_codes WHERE user_id = ?;`,
			`DELETE FROM oauth_requests WHERE user_id = ?;`,
			`DELETE FROM oauth_tokens WHERE user_id = ?;`,
		} {
			if _, err := tx.ExecContext(context.Background(), statement, userID); err != nil {
				return err
			}
		}
		result, err := tx.ExecContext(context.Background(),
			`DELETE FROM users WHERE id = ? AND id <> ?;`, userID, DeletedAccountID)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return ErrUserNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return devices, nil
}

func revokeWorkspaceDevices(tx *sql.Tx, workspaceID string) ([]string, error) {
	rows, err := tx.QueryContext(context.Background(),
		`SELECT id, identity_public_key FROM devices WHERE workspace_id = ? ORDER BY id;`, workspaceID)
	if err != nil {
		return nil, err
	}
	var devices []string
	for rows.Next() {
		var id, identity string
		if err := rows.Scan(&id, &identity); err != nil {
			rows.Close()
			return nil, err
		}
		devices = append(devices, id)
		if _, err := tx.ExecContext(context.Background(),
			`DELETE FROM authority_deliveries WHERE device_id = ?;`, id); err != nil {
			rows.Close()
			return nil, err
		}
		if _, err := tx.ExecContext(context.Background(),
			`INSERT OR REPLACE INTO revoked_devices (id, identity_public_key, revoked_ms) VALUES (?, ?, ?);`,
			id, identity, nowMs()); err != nil {
			rows.Close()
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if _, err := tx.ExecContext(context.Background(),
		`DELETE FROM devices WHERE workspace_id = ?;`, workspaceID); err != nil {
		return nil, err
	}
	return devices, nil
}
