package auth

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/OhRats-Technologies/rc/pkg/auth/webauthn"
)

func TestOpenDataDirImportsBothLegacyDatabasesOnce(t *testing.T) {
	dir := t.TempDir()
	keys := writeLegacyFixtures(t, dir)
	path := filepath.Join(dir, "rc.db")

	store, err := OpenDataDir(path)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	defer store.Close()

	if count, _ := store.UserCount(); count != 2 {
		t.Fatalf("users = %d, want 2 (tombstone excluded)", count)
	}
	passkeys, err := store.AllPasskeys()
	if err != nil || len(passkeys) != 2 {
		t.Fatalf("passkeys = %d, %v", len(passkeys), err)
	}
	byID := map[string]Passkey{}
	for _, passkey := range passkeys {
		byID[passkey.ID] = passkey
	}
	v2Key, err := webauthn.ES256CoseKey(keys.v2X, keys.v2Y)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(byID["v2-pk"].PublicKey, v2Key) || !bytes.Equal(byID["v2-pk"].CredentialID, []byte("v2-credential-id")) {
		t.Fatal("rc-v2 passkey must keep its credential ID and canonical ES256 key")
	}
	if byID["v2-pk"].Name != "Passkey" || byID["v2-pk"].Algorithm != AlgorithmES256 || byID["v2-pk"].LastUsedAt == nil {
		t.Fatalf("rc-v2 passkey metadata = %+v", byID["v2-pk"])
	}
	if !bytes.Equal(byID["relay-pk"].PublicKey, keys.relayPublicKey) || !bytes.Equal(byID["relay-pk"].CredentialID, keys.relayCredential) {
		t.Fatal("relay passkey must keep its credential ID and COSE key")
	}
	if byID["relay-pk"].Name != "Passkey" {
		t.Fatalf("relay passkey name = %q", byID["relay-pk"].Name)
	}
	user, err := store.GetUser("relay-user")
	if err != nil || user.Name != "fer" {
		t.Fatalf("relay user = %+v, %v", user, err)
	}

	devices, err := store.ListDevices()
	if err != nil || len(devices) != 2 {
		t.Fatalf("devices = %d, %v", len(devices), err)
	}
	for _, device := range devices {
		if device.ID == "relay-dev" && device.IdentityPublicKey != b64.EncodeToString(keys.relayIdentity) {
			t.Fatalf("relay device identity = %q", device.IdentityPublicKey)
		}
		if device.ID == "v2-dev" && (device.LastSeenAt == nil || device.Version != "0.19.7" || len(device.Capabilities) != 2) {
			t.Fatalf("rc-v2 device = %+v", device)
		}
	}
	revoked, err := store.RevokedDevices()
	if err != nil || len(revoked) != 2 {
		t.Fatalf("revoked = %d, %v", len(revoked), err)
	}
	if revoked[0].IdentityPublicKey != b64.EncodeToString(keys.revokedIdentity) && revoked[1].IdentityPublicKey != b64.EncodeToString(keys.revokedIdentity) {
		t.Fatal("PEM revocation key must normalize to the raw identity key")
	}

	assertLegacyFilesMoved(t, dir)
	if version := userVersion(t, path); version != schemaVersion {
		t.Fatalf("user_version = %d", version)
	}

	reopened, err := OpenDataDir(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	again, _ := reopened.ListDevices()
	if len(again) != len(devices) {
		t.Fatal("second open must not import again")
	}
}

func TestLegacyImportFailureRestoresRelayDatabase(t *testing.T) {
	dir := t.TempDir()
	writeLegacyFixtures(t, dir)
	path := filepath.Join(dir, "rc.db")
	relay := openTestDB(t, path)
	execAll(t, relay, `UPDATE devices SET public_key = 'not-a-key';`)
	relay.Close()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := OpenDataDir(path); err == nil {
		t.Fatal("corrupt legacy device key must fail the import")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed import must restore the relay database unchanged")
	}
	if _, err := os.Stat(path + legacyRelaySuffix); !os.IsNotExist(err) {
		t.Fatal("moved relay database must not remain after rollback")
	}
}

func TestOpenRejectsUnknownUnversionedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rc.db")
	db := openTestDB(t, path)
	execAll(t, db, `CREATE TABLE mystery (id TEXT);`)
	db.Close()
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Open(path); err != ErrUnversionedDatabase {
		t.Fatalf("Open = %v, want ErrUnversionedDatabase", err)
	}
}

func TestOpenDataDirWithOnlyRustEraDatabase(t *testing.T) {
	dir := t.TempDir()
	writeLegacyFixtures(t, dir)
	if err := os.Remove(filepath.Join(dir, "rc.db")); err != nil {
		t.Fatal(err)
	}
	store, err := OpenDataDir(filepath.Join(dir, "rc.db"))
	if err != nil {
		t.Fatalf("import rc-v2 only: %v", err)
	}
	defer store.Close()
	devices, _ := store.ListDevices()
	if len(devices) != 1 || devices[0].ID != "v2-dev" {
		t.Fatalf("devices = %+v", devices)
	}
}

func TestDeviceLifecycleIsDurable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rc.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureWorkspace("ws_default", "Default"); err != nil {
		t.Fatal(err)
	}
	device := Device{
		ID: "dev_1", WorkspaceID: "ws_default", Name: "box", Hostname: "box", Platform: "linux",
		Arch: "amd64", Version: "0.20.4", Capabilities: []string{"process"},
		IdentityPublicKey: b64.EncodeToString(randomKey(t)), TransportPublicKey: b64.EncodeToString(randomKey(t)),
	}
	if err := store.InsertDevice(device); err != nil {
		t.Fatal(err)
	}
	if err := store.RenameDevice("dev_1", "renamed"); err != nil {
		t.Fatal(err)
	}
	store.Close()

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	devices, err := reopened.ListDevices()
	if err != nil || len(devices) != 1 || devices[0].Name != "renamed" {
		t.Fatalf("after restart devices = %+v, %v", devices, err)
	}
	if err := reopened.RevokeDevice("dev_1"); err != nil {
		t.Fatal(err)
	}
	if revoked, _ := reopened.DeviceRevoked("dev_1"); !revoked {
		t.Fatal("revocation tombstone must persist")
	}
	if err := reopened.RevokeDevice("dev_1"); err != ErrDeviceNotFound {
		t.Fatalf("second revoke = %v", err)
	}
}

func assertLegacyFilesMoved(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, "rc.db"+legacyRelaySuffix)); err != nil {
		t.Fatalf("relay database should be kept beside the new store: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, legacyV2File)); err != nil {
		t.Fatalf("rc-v2 database must not be removed by import: %v", err)
	}
}

func userVersion(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err := db.QueryRow(`PRAGMA user_version;`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	return version
}

func TestInterruptedImportResumesFromMovedRelayDatabase(t *testing.T) {
	dir := t.TempDir()
	writeLegacyFixtures(t, dir)
	path := filepath.Join(dir, "rc.db")
	if err := moveDatabase(path, path+legacyRelaySuffix); err != nil {
		t.Fatal(err)
	}

	store, err := OpenDataDir(path)
	if err != nil {
		t.Fatalf("resume import: %v", err)
	}
	defer store.Close()
	if count, _ := store.UserCount(); count != 2 {
		t.Fatalf("users after resumed import = %d, want 2", count)
	}
}
