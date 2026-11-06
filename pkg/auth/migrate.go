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
	if version == 0 {
		_, err := s.db.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d;`, schemaVersion))
		return err
	}
	return nil
}
