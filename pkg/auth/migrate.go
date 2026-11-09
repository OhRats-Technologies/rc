package auth

import (
	"context"
	"errors"
	"fmt"
)

// ErrUnversionedDatabase reports a populated database without a schema
// version that is not a recognized legacy RC database.
var ErrUnversionedDatabase = errors.New("database has no RC schema version and is not a recognized legacy database")

func (s *Store) migrate() error {
	ctx := context.Background()
	var version int
	if err := s.db.QueryRowContext(ctx, `PRAGMA user_version;`).Scan(&version); err != nil {
		return err
	}
	switch {
	case version == schemaVersion:
	case version == 0:
		var tables int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table';`).Scan(&tables); err != nil {
			return err
		}
		if tables > 0 {
			return ErrUnversionedDatabase
		}
	case version == 1:
		// Schema 2 adds OAuth request/consent state and per-code scopes.
	case version == 2:
		// Schema 3 adds control clients, MCP grants, authority deliveries, and
		// OAuth grant links.
	default:
		return fmt.Errorf("unsupported database schema version %d (expected %d)", version, schemaVersion)
	}
	for _, statements := range [][]string{schema, oauthSchema} {
		for _, statement := range statements {
			if _, err := s.db.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
	}
	if err := s.ensureOAuthCodeScopes(ctx); err != nil {
		return err
	}
	for _, column := range []struct{ table, name, definition string }{
		{"oauth_codes", "grant_id", "TEXT NOT NULL DEFAULT ''"},
		{"oauth_requests", "prepared_grant", "TEXT NOT NULL DEFAULT ''"},
		{"oauth_tokens", "grant_id", "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := s.ensureColumn(ctx, column.table, column.name, column.definition); err != nil {
			return err
		}
	}
	if version != schemaVersion {
		_, err := s.db.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d;`, schemaVersion))
		return err
	}
	return nil
}

// ensureOAuthCodeScopes upgrades schema-1 oauth_codes tables in place. Fresh
// databases already include the column because oauthSchema creates it.
func (s *Store) ensureOAuthCodeScopes(ctx context.Context) error {
	return s.ensureColumn(ctx, "oauth_codes", "scopes", "TEXT NOT NULL DEFAULT 'mcp:observe'")
}

func (s *Store) ensureColumn(ctx context.Context, table, name, definition string) error {
	var columns int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?;`, table, name).Scan(&columns); err != nil {
		return err
	}
	if columns > 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+name+` `+definition+`;`)
	return err
}
