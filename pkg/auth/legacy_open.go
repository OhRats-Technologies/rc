package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// legacyV2File is the Rust-era database, read in place and never written.
	legacyV2File = "rc-v2.sqlite3"
	// legacyRelaySuffix names the moved relay-era database during import.
	legacyRelaySuffix = ".legacy-relay"
)

// OpenDataDir opens the RC store at path. When path is absent and a Rust-era
// rc-v2.sqlite3 sits beside it, or when path is a relay-era database, the
// legacy data is imported once into a new store at path. The legacy files are
// left in place; deleting them is an operator decision after verification.
func OpenDataDir(path string) (*Store, error) {
	dir := filepath.Dir(path)
	v2Path := filepath.Join(dir, legacyV2File)
	if _, err := os.Stat(path); err == nil {
		relay, err := isRelayDatabase(path)
		if err != nil {
			return nil, err
		}
		if !relay {
			return Open(path)
		}
		relayPath := path + legacyRelaySuffix
		if err := moveDatabase(path, relayPath); err != nil {
			return nil, fmt.Errorf("move relay database aside: %w", err)
		}
		return importLegacy(path, relayPath, existingFile(v2Path))
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	// A moved relay database without a store means an earlier import stopped
	// before it committed, so the relay data is still pending.
	relayPath := ""
	if existingFile(path+legacyRelaySuffix) != "" {
		relayPath = path + legacyRelaySuffix
	}
	return importLegacy(path, relayPath, existingFile(v2Path))
}

// importLegacy creates a fresh store at path and imports the given sources.
// On failure it removes the partial store and restores the relay database.
func importLegacy(path, relayPath, v2Path string) (*Store, error) {
	fail := func(err error) (*Store, error) {
		removeDatabase(path)
		if relayPath != "" {
			if restoreErr := moveDatabase(relayPath, path); restoreErr != nil {
				err = errors.Join(err, fmt.Errorf("restore relay database: %w", restoreErr))
			}
		}
		return nil, fmt.Errorf("legacy import failed: %w", err)
	}
	data, err := readLegacySources(relayPath, v2Path)
	if err != nil {
		return fail(err)
	}
	store, err := Open(path)
	if err != nil {
		return fail(err)
	}
	if err := store.importData(data); err != nil {
		store.Close()
		return fail(err)
	}
	return store, nil
}

func readLegacySources(relayPath, v2Path string) (legacyData, error) {
	var data legacyData
	if relayPath != "" {
		if err := readRelay(relayPath, &data); err != nil {
			return legacyData{}, fmt.Errorf("relay database: %w", err)
		}
	}
	if v2Path != "" {
		if err := readV2(v2Path, &data); err != nil {
			return legacyData{}, fmt.Errorf("rc-v2 database: %w", err)
		}
	}
	return data, nil
}

// isRelayDatabase reports whether path is an unversioned database with the
// relay-era users table, which the current schema cannot open in place.
func isRelayDatabase(path string) (bool, error) {
	db, err := openReadOnly(path)
	if err != nil {
		return false, err
	}
	defer db.Close()
	var version int
	if err := db.QueryRowContext(context.Background(), `PRAGMA user_version;`).Scan(&version); err != nil {
		return false, err
	}
	if version != 0 {
		return false, nil
	}
	var columns int
	err = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name = 'created_at';`).Scan(&columns)
	if err != nil {
		return false, err
	}
	return columns > 0, nil
}

func openReadOnly(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

var databaseSuffixes = []string{"", "-wal", "-shm"}

func moveDatabase(from, to string) error {
	for _, suffix := range databaseSuffixes {
		if _, err := os.Stat(from + suffix); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := os.Rename(from+suffix, to+suffix); err != nil {
			return err
		}
	}
	return nil
}

func removeDatabase(path string) {
	for _, suffix := range databaseSuffixes {
		_ = os.Remove(path + suffix)
	}
}

func existingFile(path string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}
