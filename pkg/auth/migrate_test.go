package auth

import (
	"context"
	"os"
	"testing"
)

func TestSchemaOneAddsOAuthRequestStateAndCodeScopes(t *testing.T) {
	path := t.TempDir() + "/rc.db"
	db := openTestDB(t, path)
	execAll(t, db, `CREATE TABLE oauth_codes (
		code_hash BLOB PRIMARY KEY,
		client_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		redirect_uri TEXT NOT NULL,
		code_challenge TEXT NOT NULL,
		resource TEXT NOT NULL,
		expires_ms INTEGER NOT NULL
	);`)
	if _, err := db.Exec(`PRAGMA user_version = 1;`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatalf("open schema 1: %v", err)
	}
	defer store.Close()
	var version int
	if err := store.db.QueryRowContext(context.Background(), `PRAGMA user_version;`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Fatalf("user_version = %d, want %d", version, schemaVersion)
	}
	var scopes int
	if err := store.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM pragma_table_info('oauth_codes') WHERE name = 'scopes';`).Scan(&scopes); err != nil {
		t.Fatal(err)
	}
	if scopes != 1 {
		t.Fatal("oauth_codes.scopes was not added")
	}
	var requests int
	if err := store.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'oauth_requests';`).Scan(&requests); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatal("oauth_requests table was not created")
	}
}

func TestSchemaThreeAddsAPIKeyAuthorityTables(t *testing.T) {
	path := t.TempDir() + "/rc.db"
	db := openTestDB(t, path)
	if _, err := db.Exec(`PRAGMA user_version = 3;`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatalf("open schema 3: %v", err)
	}
	defer store.Close()
	for _, table := range []string{"api_keys", "schedule_grants", "request_nonces"} {
		var count int
		if err := store.db.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?;`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s table was not created", table)
		}
	}
}
