package platform

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// DB wraps an underlying SQL database connection with RC security and pragmas.
type DB struct {
	*sql.DB
	path string
}

// OpenDB opens a SQLite database at the specified path, ensuring parent permissions and WAL pragmas.
func OpenDB(path string) (*DB, error) {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return nil, err
	}
	if err := ProtectPrivatePath(parent, true); err != nil {
		return nil, err
	}

	if _, err := os.Stat(path); err == nil {
		if err := ValidatePrivatePath(path, false); err != nil {
			return nil, err
		}
	}

	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=FULL;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA wal_autocheckpoint=1000;",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("execute pragma %q: %w", pragma, err)
		}
	}

	// Protect file and related WAL/SHM files
	if err := secureDBFiles(path); err != nil {
		db.Close()
		return nil, err
	}

	return &DB{DB: db, path: path}, nil
}

func secureDBFiles(path string) error {
	_ = ProtectPrivatePath(path, false)
	for _, ext := range []string{"-wal", "-shm"} {
		sidecar := path + ext
		if _, err := os.Stat(sidecar); err == nil {
			_ = ProtectPrivatePath(sidecar, false)
		}
	}
	return nil
}

// IntegrityCheck verifies the database integrity and foreign key validity.
func (db *DB) IntegrityCheck(ctx context.Context) error {
	var quickCheck string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check;").Scan(&quickCheck); err != nil {
		return err
	}
	if quickCheck != "ok" {
		return fmt.Errorf("sqlite quick_check failed: %s", quickCheck)
	}

	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check;")
	if err != nil {
		return err
	}
	defer rows.Close()

	var violations []string
	for rows.Next() {
		var table, parent string
		var rowid, fkid int64
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err == nil {
			violations = append(violations, fmt.Sprintf("%s(rowid=%d)->%s", table, rowid, parent))
		}
	}
	if len(violations) > 0 {
		return fmt.Errorf("foreign key violations: %s", strings.Join(violations, ", "))
	}
	return nil
}

// WithTx runs fn within an immediate database transaction.
func (db *DB) WithTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_ = secureDBFiles(db.path)
	return nil
}
