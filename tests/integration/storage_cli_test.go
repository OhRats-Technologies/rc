package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIStoragePersistsAcrossProcesses(t *testing.T) {
	root := repoRoot(t)
	binary := buildBinary(t, "./cmd/rc", "rc")
	components := componentArtifacts(t, "storage-fixture")
	dataDir := t.TempDir()
	env := []string{"RC_DATA_DIR=" + dataDir}

	stdout, stderr, err := runCommand(t, root, env, binary, "--component-dir", components, "kv-set", "bucket", "key", "value")
	if err != nil {
		t.Fatalf("kv-set: %v\n%s%s", err, stdout, stderr)
	}
	stdout, stderr, err = runCommand(t, root, env, binary, "--component-dir", components, "kv-get", "bucket", "key")
	if err != nil {
		t.Fatalf("kv-get: %v\n%s%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "value") {
		t.Fatalf("kv-get output = %q", stdout)
	}
	stdout, stderr, err = runCommand(t, root, env, binary, "--component-dir", components, "kv-list", "bucket")
	if err != nil {
		t.Fatalf("kv-list: %v\n%s%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "key=value") {
		t.Fatalf("kv-list output = %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "components.db")); err != nil {
		t.Fatalf("components.db missing: %v", err)
	}
}
