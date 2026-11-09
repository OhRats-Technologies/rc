package auth

import (
	"path/filepath"
	"testing"
)

const legacyMCPDDL = `
CREATE TABLE mcp_clients(id TEXT PRIMARY KEY,name TEXT NOT NULL,redirect_uris TEXT NOT NULL,created_at INTEGER NOT NULL);
CREATE TABLE mcp_grants(id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,client_id TEXT NOT NULL REFERENCES mcp_clients(id) ON DELETE CASCADE,name TEXT NOT NULL,grant TEXT NOT NULL,grant_signature TEXT NOT NULL,client_control_id TEXT NOT NULL,credential_id TEXT NOT NULL,control_grant TEXT NOT NULL,control_assertion TEXT NOT NULL,created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL,last_used INTEGER,revoked_at INTEGER);`

func writeLegacyMCP(t *testing.T, path, userID string) {
	t.Helper()
	db := openTestDB(t, path)
	execAll(t, db, legacyMCPDDL,
		`INSERT INTO mcp_clients VALUES ('mcp_client_legacy', 'Gemini', '["https://oauth-redirect.googleusercontent.com/callback"]', 1787857839710);`,
		`INSERT INTO mcp_grants VALUES ('mcp_legacy', '`+userID+`', 'mcp_client_legacy', 'Gemini', '{}', 'signature', 'control', 'credential', 'control-grant', 'assertion', 1787857839710, 0, NULL, NULL);`,
	)
	db.Close()
}

func TestLegacyMCPClientsAndGrantsImportWithTheAccountStore(t *testing.T) {
	dir := t.TempDir()
	writeLegacyFixtures(t, dir)
	writeLegacyMCP(t, filepath.Join(dir, legacyV2File), "v2-user")
	store, err := OpenDataDir(filepath.Join(dir, "rc.db"))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	defer store.Close()
	client, err := store.GetOAuthClient("mcp_client_legacy")
	if err != nil || client.Name != "Gemini" || len(client.RedirectURIs) != 1 {
		t.Fatalf("legacy MCP client = %+v, %v", client, err)
	}
	grant, err := store.McpGrant("v2-user", "mcp_legacy")
	if err != nil || grant.ClientID != "mcp_client_legacy" || grant.Signature != "signature" {
		t.Fatalf("legacy MCP grant = %+v, %v", grant, err)
	}
}

func TestLegacyMCPBackfillRepairsAnAlreadyMigratedStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rc.db")
	store, err := OpenDataDir(path)
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUserWithID("legacy-user", "Legacy")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	writeLegacyMCP(t, filepath.Join(dir, legacyV2File), user.ID)

	reopened, err := OpenDataDir(path)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	defer reopened.Close()
	if _, err := reopened.GetOAuthClient("mcp_client_legacy"); err != nil {
		t.Fatalf("backfilled MCP client: %v", err)
	}
	if _, err := reopened.McpGrant(user.ID, "mcp_legacy"); err != nil {
		t.Fatalf("backfilled MCP grant: %v", err)
	}
}
