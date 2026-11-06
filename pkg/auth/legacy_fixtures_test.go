package auth

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"path/filepath"
	"testing"

	"github.com/OhRats-Technologies/rc/pkg/auth/webauthn"
)

// v2DDL and relayDDL mirror the table shapes of the two legacy databases.
const v2DDL = `
CREATE TABLE users(id TEXT PRIMARY KEY,name TEXT NOT NULL,created_at INTEGER NOT NULL);
CREATE TABLE passkeys(id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,name TEXT NOT NULL,credential_json TEXT NOT NULL,created_at INTEGER NOT NULL,last_used INTEGER);
CREATE TABLE workspaces(id TEXT PRIMARY KEY,name TEXT NOT NULL,created_by TEXT NOT NULL REFERENCES users(id),created_at INTEGER NOT NULL);
CREATE TABLE workspace_members(workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,role TEXT NOT NULL CHECK(role IN ('owner','operator','viewer')),joined_at INTEGER NOT NULL,PRIMARY KEY(workspace_id,user_id));
CREATE TABLE devices(id TEXT PRIMARY KEY,workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,name TEXT NOT NULL,hostname TEXT NOT NULL,platform TEXT NOT NULL,arch TEXT NOT NULL,identity_public_key TEXT NOT NULL UNIQUE,transport_public_key TEXT NOT NULL,lock_hash TEXT NOT NULL DEFAULT '',lock_generation INTEGER NOT NULL DEFAULT 0,version TEXT NOT NULL,capabilities TEXT NOT NULL DEFAULT '[]',last_seen INTEGER,created_at INTEGER NOT NULL);
CREATE TABLE revoked_devices(id TEXT PRIMARY KEY,identity_public_key TEXT NOT NULL,revoked_at INTEGER NOT NULL);`

const relayDDL = `
CREATE TABLE users(id TEXT PRIMARY KEY,name TEXT NOT NULL,created_at INTEGER NOT NULL);
CREATE TABLE passkeys(id TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,credential_id TEXT NOT NULL UNIQUE,public_key TEXT NOT NULL,counter INTEGER NOT NULL DEFAULT 0,transports TEXT NOT NULL DEFAULT '[]',created_at INTEGER NOT NULL,last_used INTEGER);
CREATE TABLE workspaces(id TEXT PRIMARY KEY,name TEXT NOT NULL,created_by TEXT NOT NULL REFERENCES users(id),created_at INTEGER NOT NULL);
CREATE TABLE workspace_members(workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,role TEXT NOT NULL CHECK(role IN ('owner','operator','viewer')),joined_at INTEGER NOT NULL,PRIMARY KEY(workspace_id,user_id));
CREATE TABLE devices(id TEXT PRIMARY KEY,workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,name TEXT NOT NULL,hostname TEXT NOT NULL,platform TEXT NOT NULL,arch TEXT NOT NULL,public_key TEXT NOT NULL UNIQUE,agent_version TEXT NOT NULL,capabilities TEXT NOT NULL DEFAULT '[]',last_seen INTEGER,created_at INTEGER NOT NULL,transport_public_key TEXT NOT NULL DEFAULT '',lock_hash TEXT NOT NULL DEFAULT '',lock_generation INTEGER NOT NULL DEFAULT 0);
CREATE TABLE revoked_devices(id TEXT PRIMARY KEY,public_key TEXT NOT NULL,revoked_at INTEGER NOT NULL);`

var b64 = base64.RawURLEncoding

func randomKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key
}

// p256Point returns a fresh P-256 key as 32-byte X and Y coordinates.
func p256Point(t *testing.T) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key.PublicKey.X.FillBytes(make([]byte, 32)), key.PublicKey.Y.FillBytes(make([]byte, 32))
}

// spkiPEM encodes a raw Ed25519 key the way the relay-era database stored it.
func spkiPEM(raw []byte) string {
	der := append(append([]byte{}, ed25519SPKIPrefix...), raw...)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func openTestDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func execAll(t *testing.T, db *sql.DB, statements ...string) {
	t.Helper()
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("exec %q: %v", statement, err)
		}
	}
}

type fixtureKeys struct {
	v2X, v2Y        []byte
	v2Identity      []byte
	relayIdentity   []byte
	relayTransport  []byte
	relayCredential []byte
	relayPublicKey  []byte
	revokedIdentity []byte
}

// writeLegacyFixtures creates rc-v2.sqlite3 and rc.db under dir with one
// account, passkey, workspace, and device in each, plus a tombstone account.
func writeLegacyFixtures(t *testing.T, dir string) fixtureKeys {
	t.Helper()
	var keys fixtureKeys
	keys.v2X, keys.v2Y = p256Point(t)
	keys.v2Identity = randomKey(t)
	keys.relayIdentity = randomKey(t)
	keys.relayTransport = randomKey(t)
	keys.revokedIdentity = randomKey(t)
	relayX, relayY := p256Point(t)
	keys.relayCredential = []byte("relay-credential-id-16")
	cose, err := webauthn.ES256CoseKey(relayX, relayY)
	if err != nil {
		t.Fatal(err)
	}
	keys.relayPublicKey = cose

	document, _ := json.Marshal(map[string]any{"cred": map[string]any{
		"cred_id": b64.EncodeToString([]byte("v2-credential-id")),
		"cred": map[string]any{
			"type_": "ES256",
			"key": map[string]any{"EC_EC2": map[string]any{
				"curve": "SECP256R1", "x": b64.EncodeToString(keys.v2X), "y": b64.EncodeToString(keys.v2Y),
			}},
		},
	}})

	v2 := openTestDB(t, filepath.Join(dir, "rc-v2.sqlite3"))
	execAll(t, v2, v2DDL,
		`INSERT INTO users VALUES ('v2-user', 'Ada', 1787857839709);`,
		`INSERT INTO passkeys VALUES ('v2-pk', 'v2-user', 'Passkey', '`+string(document)+`', 1787857839710, 1792985052947);`,
		`INSERT INTO workspaces VALUES ('v2-ws', 'Personal', 'v2-user', 1787857839710);`,
		`INSERT INTO workspace_members VALUES ('v2-ws', 'v2-user', 'owner', 1787857839710);`,
		`INSERT INTO devices VALUES ('v2-dev', 'v2-ws', 'fermac', 'mac.local', 'darwin', 'arm64', '`+
			b64.EncodeToString(keys.v2Identity)+`', '`+b64.EncodeToString(randomKey(t))+
			`', '', 5, '0.19.7', '["process","lock"]', 1788134563857, 1787857839710);`,
		`INSERT INTO revoked_devices VALUES ('v2-gone', '`+b64.EncodeToString(keys.revokedIdentity)+`', 1790531164146);`,
	)
	v2.Close()

	relay := openTestDB(t, filepath.Join(dir, "rc.db"))
	execAll(t, relay, relayDDL,
		`INSERT INTO users VALUES ('__deleted_account__', 'Deleted account', 1787690334159);`,
		`INSERT INTO users VALUES ('relay-user', 'fer', 1787690387392);`,
		`INSERT INTO passkeys VALUES ('relay-pk', 'relay-user', '`+b64.EncodeToString(keys.relayCredential)+
			`', '`+b64.EncodeToString(cose)+`', 0, '[]', 1787690387394, NULL);`,
		`INSERT INTO workspaces VALUES ('relay-ws', 'Personal', 'relay-user', 1787690387392);`,
		`INSERT INTO workspace_members VALUES ('relay-ws', 'relay-user', 'owner', 1787690387392);`,
		`INSERT INTO devices VALUES ('relay-dev', 'relay-ws', 'beamforming', 'beamforming', 'linux', 'arm64',
			'`+escapeSQL(spkiPEM(keys.relayIdentity))+`', '0.9.2', '["process"]', 1787716920888, 1787704951019,
			'`+b64.EncodeToString(keys.relayTransport)+`', '', 5);`,
		`INSERT INTO revoked_devices VALUES ('relay-gone', '`+escapeSQL(spkiPEM(keys.revokedIdentity))+`', 1787800000000);`,
	)
	relay.Close()
	return keys
}

func escapeSQL(value string) string {
	var out bytes.Buffer
	for _, r := range value {
		if r == '\'' {
			out.WriteString("''")
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}
