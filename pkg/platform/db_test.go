package platform

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenDBAndIntegrityCheck(t *testing.T) {
	temp := t.TempDir()
	dbPath := filepath.Join(temp, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.IntegrityCheck(ctx); err != nil {
		t.Fatalf("IntegrityCheck failed: %v", err)
	}

	err = db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("CREATE TABLE kv (k TEXT PRIMARY KEY, v TEXT);")
		if err != nil {
			return err
		}
		_, err = tx.Exec("INSERT INTO kv (k, v) VALUES (?, ?);", "hello", "world")
		return err
	})
	if err != nil {
		t.Fatalf("WithTx failed: %v", err)
	}

	var val string
	err = db.QueryRowContext(ctx, "SELECT v FROM kv WHERE k = ?", "hello").Scan(&val)
	if err != nil || val != "world" {
		t.Fatalf("query failed: val=%s, err=%v", val, err)
	}

	if err := db.IntegrityCheck(ctx); err != nil {
		t.Fatalf("IntegrityCheck after write failed: %v", err)
	}
}
