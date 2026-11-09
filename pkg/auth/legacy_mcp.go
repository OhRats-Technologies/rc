package auth

import (
	"context"
	"database/sql"
)

// legacyMCPClient is one Rust-era dynamic MCP client registration.
type legacyMCPClient struct {
	ID           string
	Name         string
	RedirectURIs string
	CreatedAtMs  int64
}

// legacyMCPGrant is one Rust-era Owner-authorized MCP grant.
type legacyMCPGrant struct {
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
	CreatedAtMs      int64
	ExpiresAtMs      int64
	RevokedAtMs      sql.NullInt64
}

// readV2MCP reads the long-lived MCP client and grant rows. Legacy OAuth
// tokens and authorization codes use a different hash encoding and are not
// imported; clients re-run OAuth and receive current-format tokens.
func readV2MCP(ctx context.Context, db *sql.DB, data *legacyData) error {
	clients, err := tableExists(ctx, db, "mcp_clients")
	if err != nil {
		return err
	}
	if clients {
		rows, err := db.QueryContext(ctx,
			`SELECT id, name, redirect_uris, created_at FROM mcp_clients ORDER BY id;`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var client legacyMCPClient
			if err := rows.Scan(&client.ID, &client.Name, &client.RedirectURIs, &client.CreatedAtMs); err != nil {
				rows.Close()
				return err
			}
			data.mcpClients = append(data.mcpClients, client)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	grants, err := tableExists(ctx, db, "mcp_grants")
	if err != nil {
		return err
	}
	if grants {
		rows, err := db.QueryContext(ctx,
			`SELECT id, user_id, client_id, name, grant, grant_signature, client_control_id,
			        credential_id, control_grant, control_assertion, created_at, expires_at,
			        last_used, revoked_at
			 FROM mcp_grants ORDER BY id;`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var grant legacyMCPGrant
			var lastUsed sql.NullInt64
			if err := rows.Scan(&grant.ID, &grant.UserID, &grant.ClientID, &grant.Name,
				&grant.Grant, &grant.Signature, &grant.ControlClientID, &grant.CredentialID,
				&grant.ControlGrant, &grant.ControlAssertion, &grant.CreatedAtMs,
				&grant.ExpiresAtMs, &lastUsed, &grant.RevokedAtMs); err != nil {
				rows.Close()
				return err
			}
			data.mcpGrants = append(data.mcpGrants, grant)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	return nil
}

func tableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?;`, name).Scan(&count)
	return count > 0, err
}

// backfillLegacyMCP imports only missing MCP client and grant rows into an
// already-created Go store. This makes a deployment that migrated before MCP
// migration existed recover cached client registrations on the next start.
func (s *Store) backfillLegacyMCP(path string) error {
	db, err := openReadOnly(path)
	if err != nil {
		return err
	}
	defer db.Close()
	var data legacyData
	if err := readV2MCP(context.Background(), db, &data); err != nil {
		return err
	}
	if len(data.mcpClients) == 0 && len(data.mcpGrants) == 0 {
		return nil
	}
	return s.db.WithTx(context.Background(), func(tx *sql.Tx) error {
		ctx := context.Background()
		for _, client := range data.mcpClients {
			if _, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO oauth_clients (client_id, client_name, redirect_uris, created_ms)
				 VALUES (?, ?, ?, ?);`,
				client.ID, client.Name, client.RedirectURIs, client.CreatedAtMs); err != nil {
				return err
			}
		}
		for _, grant := range data.mcpGrants {
			var exists int
			if err := tx.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM users WHERE id = ?;`, grant.UserID).Scan(&exists); err != nil {
				return err
			}
			if exists == 0 {
				continue
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO mcp_grants
				 (id, user_id, client_id, name, grant, signature, control_client_id,
				  credential_id, control_grant, control_assertion, created_ms, expires_ms, revoked_ms)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`,
				grant.ID, grant.UserID, grant.ClientID, grant.Name, grant.Grant, grant.Signature,
				grant.ControlClientID, grant.CredentialID, grant.ControlGrant, grant.ControlAssertion,
				grant.CreatedAtMs, grant.ExpiresAtMs, nullableMsFromInt(grant.RevokedAtMs)); err != nil {
				return err
			}
		}
		return nil
	})
}

func nullableMsFromInt(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}
