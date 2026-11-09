package auth

import (
	"context"
	"database/sql"
	"errors"

	"github.com/OhRats-Technologies/rc/pkg/auth/webauthn"
)

// relayDeletedAccount is the tombstone user the relay wrote for deleted accounts.
const relayDeletedAccount = DeletedAccountID

// readRelay reads the relay-era database. Its passkeys have no display name,
// so they receive the default name used by the current setup flow.
func readRelay(path string, data *legacyData) error {
	db, err := openReadOnly(path)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx := context.Background()
	users, err := readUsers(ctx, db,
		`SELECT id, name, created_at FROM users WHERE id <> ? ORDER BY id;`, relayDeletedAccount)
	if err != nil {
		return err
	}
	data.users = append(data.users, users...)
	if err := readRelayPasskeys(ctx, db, data); err != nil {
		return err
	}
	if err := readWorkspaces(ctx, db, data); err != nil {
		return err
	}
	devices, err := readDevices(ctx, db, `SELECT id, workspace_id, name, hostname, platform, arch,
		agent_version, capabilities, public_key, transport_public_key, lock_hash, lock_generation,
		last_seen, created_at FROM devices ORDER BY id;`)
	if err != nil {
		return err
	}
	data.devices = append(data.devices, devices...)
	revoked, err := readRevoked(ctx, db, `SELECT id, public_key, revoked_at FROM revoked_devices ORDER BY id;`)
	if err != nil {
		return err
	}
	data.revoked = append(data.revoked, revoked...)
	return nil
}

func readRelayPasskeys(ctx context.Context, db *sql.DB, data *legacyData) error {
	rows, err := db.QueryContext(ctx,
		`SELECT id, user_id, credential_id, public_key, created_at, last_used FROM passkeys ORDER BY id;`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, userID, credentialID, publicKey string
		var createdMs int64
		var lastUsed sql.NullInt64
		if err := rows.Scan(&id, &userID, &credentialID, &publicKey, &createdMs, &lastUsed); err != nil {
			return err
		}
		credential, err := decodeLegacyBytes(credentialID)
		if err != nil {
			return errors.New("passkey " + id + ": credential id is not base64")
		}
		raw, err := decodeLegacyBytes(publicKey)
		if err != nil {
			return errors.New("passkey " + id + ": public key is not base64")
		}
		key, err := webauthn.CanonicalCoseKey(raw)
		if err != nil {
			return errors.New("passkey " + id + ": " + err.Error())
		}
		passkey := Passkey{
			ID:           id,
			UserID:       userID,
			Name:         "Passkey",
			CredentialID: credential,
			PublicKey:    key,
			Algorithm:    AlgorithmES256,
			CreatedAt:    timeFromMs(createdMs),
		}
		if lastUsed.Valid {
			used := timeFromMs(lastUsed.Int64)
			passkey.LastUsedAt = &used
		}
		data.passkeys = append(data.passkeys, passkey)
	}
	return rows.Err()
}

func readUsers(ctx context.Context, db *sql.DB, query string, args ...any) ([]User, error) {
	rows, err := db.QueryContext(ctx, query, args...)
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

func readWorkspaces(ctx context.Context, db *sql.DB, data *legacyData) error {
	rows, err := db.QueryContext(ctx,
		`SELECT id, name, created_by, created_at FROM workspaces ORDER BY id;`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var workspace Workspace
		var createdBy sql.NullString
		var createdMs int64
		if err := rows.Scan(&workspace.ID, &workspace.Name, &createdBy, &createdMs); err != nil {
			rows.Close()
			return err
		}
		workspace.CreatedBy = createdBy.String
		workspace.CreatedAt = timeFromMs(createdMs)
		data.workspaces = append(data.workspaces, workspace)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	members, err := db.QueryContext(ctx,
		`SELECT workspace_id, user_id, role, joined_at FROM workspace_members ORDER BY workspace_id, user_id;`)
	if err != nil {
		return err
	}
	defer members.Close()
	for members.Next() {
		var membership Membership
		var joinedMs int64
		if err := members.Scan(&membership.WorkspaceID, &membership.UserID, &membership.Role, &joinedMs); err != nil {
			return err
		}
		membership.JoinedAt = timeFromMs(joinedMs)
		data.memberships = append(data.memberships, membership)
	}
	return members.Err()
}
