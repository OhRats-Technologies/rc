package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// legacyData is the normalized content read from the legacy databases.
type legacyData struct {
	users       []User
	passkeys    []Passkey
	workspaces  []Workspace
	memberships []Membership
	devices     []Device
	revoked     []RevokedDevice
	mcpClients  []legacyMCPClient
	mcpGrants   []legacyMCPGrant
}

// readDevices scans device rows selected in the device-column order used by
// every legacy schema, normalizing keys to base64url.
func readDevices(ctx context.Context, db *sql.DB, query string) ([]Device, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var devices []Device
	for rows.Next() {
		var device Device
		var capabilities, identity, transport string
		var lastSeen sql.NullInt64
		var createdMs int64
		if err := rows.Scan(&device.ID, &device.WorkspaceID, &device.Name, &device.Hostname,
			&device.Platform, &device.Arch, &device.Version, &capabilities, &identity, &transport,
			&device.LockHash, &device.LockGeneration, &lastSeen, &createdMs); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(capabilities), &device.Capabilities); err != nil {
			return nil, fmt.Errorf("device %s: capabilities: %w", device.ID, err)
		}
		if device.IdentityPublicKey, err = normalizeKey(identity); err != nil {
			return nil, fmt.Errorf("device %s: identity key: %w", device.ID, err)
		}
		if device.TransportPublicKey, err = normalizeKey(transport); err != nil {
			return nil, fmt.Errorf("device %s: transport key: %w", device.ID, err)
		}
		if lastSeen.Valid {
			seen := timeFromMs(lastSeen.Int64)
			device.LastSeenAt = &seen
		}
		device.CreatedAt = timeFromMs(createdMs)
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

func readRevoked(ctx context.Context, db *sql.DB, query string) ([]RevokedDevice, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var revoked []RevokedDevice
	for rows.Next() {
		var item RevokedDevice
		var identity string
		var revokedMs int64
		if err := rows.Scan(&item.ID, &identity, &revokedMs); err != nil {
			return nil, err
		}
		if item.IdentityPublicKey, err = normalizeKey(identity); err != nil {
			return nil, fmt.Errorf("revoked device %s: identity key: %w", item.ID, err)
		}
		item.RevokedAt = timeFromMs(revokedMs)
		revoked = append(revoked, item)
	}
	return revoked, rows.Err()
}

// importData writes every legacy row in one transaction. Foreign-key violations
// or constraint failures roll back the whole import.
func (s *Store) importData(data legacyData) error {
	return s.db.WithTx(context.Background(), func(tx *sql.Tx) error {
		ctx := context.Background()
		for _, user := range data.users {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO users (id, name, created_ms) VALUES (?, ?, ?);`,
				user.ID, user.Name, msFromTime(user.CreatedAt)); err != nil {
				return fmt.Errorf("user %s: %w", user.ID, err)
			}
		}
		for _, passkey := range data.passkeys {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO passkeys (id, user_id, name, credential_id, public_key, algorithm, created_ms, last_used_ms)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?);`,
				passkey.ID, passkey.UserID, passkey.Name, passkey.CredentialID, passkey.PublicKey,
				passkey.Algorithm, msFromTime(passkey.CreatedAt), nullableMs(passkey.LastUsedAt)); err != nil {
				return fmt.Errorf("passkey %s: %w", passkey.ID, err)
			}
		}
		for _, workspace := range data.workspaces {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO workspaces (id, name, created_by, created_ms) VALUES (?, ?, ?, ?);`,
				workspace.ID, workspace.Name, nullableString(workspace.CreatedBy), msFromTime(workspace.CreatedAt)); err != nil {
				return fmt.Errorf("workspace %s: %w", workspace.ID, err)
			}
		}
		for _, membership := range data.memberships {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO workspace_members (workspace_id, user_id, role, joined_ms) VALUES (?, ?, ?, ?);`,
				membership.WorkspaceID, membership.UserID, membership.Role, msFromTime(membership.JoinedAt)); err != nil {
				return fmt.Errorf("membership %s/%s: %w", membership.WorkspaceID, membership.UserID, err)
			}
		}
		for _, device := range data.devices {
			if _, err := tx.ExecContext(ctx, deviceInsertSQL, deviceInsertArgs(device)...); err != nil {
				return fmt.Errorf("device %s: %w", device.ID, err)
			}
		}
		for _, item := range data.revoked {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO revoked_devices (id, identity_public_key, revoked_ms) VALUES (?, ?, ?);`,
				item.ID, item.IdentityPublicKey, msFromTime(item.RevokedAt)); err != nil {
				return fmt.Errorf("revoked device %s: %w", item.ID, err)
			}
		}
		for _, client := range data.mcpClients {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO oauth_clients (client_id, client_name, redirect_uris, created_ms)
				 VALUES (?, ?, ?, ?);`,
				client.ID, client.Name, client.RedirectURIs, client.CreatedAtMs); err != nil {
				return fmt.Errorf("MCP client %s: %w", client.ID, err)
			}
		}
		for _, grant := range data.mcpGrants {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO mcp_grants
				 (id, user_id, client_id, name, grant, signature, control_client_id,
				  credential_id, control_grant, control_assertion, created_ms, expires_ms, revoked_ms)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`,
				grant.ID, grant.UserID, grant.ClientID, grant.Name, grant.Grant, grant.Signature,
				grant.ControlClientID, grant.CredentialID, grant.ControlGrant, grant.ControlAssertion,
				grant.CreatedAtMs, grant.ExpiresAtMs, nullableMsFromInt(grant.RevokedAtMs)); err != nil {
				return fmt.Errorf("MCP grant %s: %w", grant.ID, err)
			}
		}
		rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check;`)
		if err != nil {
			return err
		}
		defer rows.Close()
		if rows.Next() {
			return fmt.Errorf("foreign key violation after import")
		}
		return rows.Err()
	})
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
