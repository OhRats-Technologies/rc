//go:build cgo && !windows

package capi

import (
	"context"
	"database/sql"
	"errors"

	"github.com/OhRats-Technologies/rc/pkg/platform"
)

type sqliteStorage struct {
	db *platform.DB
}

func openSQLiteStorage(path string) (*sqliteStorage, error) {
	db, err := platform.OpenDB(path)
	if err != nil {
		return nil, err
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS component_storage (
			bucket TEXT NOT NULL,
			key BLOB NOT NULL,
			value BLOB NOT NULL,
			PRIMARY KEY (bucket, key)
		);`,
		`CREATE TABLE IF NOT EXISTS component_storage_revision (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			revision INTEGER NOT NULL
		);`,
		`INSERT OR IGNORE INTO component_storage_revision (id, revision) VALUES (1, 0);`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(context.Background(), statement); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &sqliteStorage{db: db}, nil
}

func (s *sqliteStorage) revision() (uint64, error) {
	var revision int64
	if err := s.db.QueryRowContext(context.Background(),
		`SELECT revision FROM component_storage_revision WHERE id = 1;`).Scan(&revision); err != nil {
		return 0, err
	}
	return uint64(revision), nil
}

func (s *sqliteStorage) get(bucket string, key []byte) ([]byte, bool, error) {
	var value []byte
	err := s.db.QueryRowContext(context.Background(),
		`SELECT value FROM component_storage WHERE bucket = ? AND key = ?;`, bucket, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func (s *sqliteStorage) scan(bucket string, prefix, after []byte, limit int) (uint64, []entryValue, []byte, error) {
	revision, err := s.revision()
	if err != nil {
		return 0, nil, nil, err
	}
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT key, value FROM component_storage WHERE bucket = ? ORDER BY key;`, bucket)
	if err != nil {
		return 0, nil, nil, err
	}
	defer rows.Close()
	var entries []entryValue
	var next []byte
	for rows.Next() {
		var key, value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return 0, nil, nil, err
		}
		if len(prefix) > 0 && !hasPrefix(key, prefix) {
			continue
		}
		if after != nil && !greater(key, after) {
			continue
		}
		if len(entries) == limit {
			next = append([]byte(nil), key...)
			break
		}
		entries = append(entries, entryValue{bucket: bucket, key: append([]byte(nil), key...), value: append([]byte(nil), value...)})
	}
	return revision, entries, next, rows.Err()
}

func (s *sqliteStorage) commit(expected uint64, changes []storageChange) (uint64, *storageFailure, error) {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback()
	var revision int64
	if err := tx.QueryRowContext(ctx,
		`SELECT revision FROM component_storage_revision WHERE id = 1;`).Scan(&revision); err != nil {
		return 0, nil, err
	}
	if uint64(revision) != expected {
		return uint64(revision), &storageFailure{revision: uint64(revision)}, nil
	}
	for _, change := range changes {
		if change.put {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO component_storage (bucket, key, value) VALUES (?, ?, ?)
				 ON CONFLICT (bucket, key) DO UPDATE SET value = excluded.value;`,
				change.bucket, change.key, change.value); err != nil {
				return 0, nil, err
			}
		} else {
			if _, err := tx.ExecContext(ctx,
				`DELETE FROM component_storage WHERE bucket = ? AND key = ?;`,
				change.bucket, change.key); err != nil {
				return 0, nil, err
			}
		}
	}
	next := uint64(revision) + 1
	if _, err := tx.ExecContext(ctx,
		`UPDATE component_storage_revision SET revision = ? WHERE id = 1;`, next); err != nil {
		return 0, nil, err
	}
	if err := tx.Commit(); err != nil {
		return 0, nil, err
	}
	return next, nil, nil
}

func (s *sqliteStorage) close() error {
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}
