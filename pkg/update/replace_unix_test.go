//go:build !windows

package update

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OhRats-Technologies/rc/pkg/platform"
)

func TestCommitBinaryReplacesAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, platform.ExecutableName("rc"))
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	staged, err := stageBinary(dir, []byte("new"))
	if err != nil {
		t.Fatal(err)
	}
	if err := commitBinary(dir, staged, Digest([]byte("new")), "0.0.0"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "new" {
		t.Fatalf("target = %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dir, replacementJournal)); !os.IsNotExist(err) {
		t.Fatal("replacement journal was not removed")
	}
}

func TestRecoverPendingRestoresBackup(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, platform.ExecutableName("rc"))
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(dir, "stage")
	backup := filepath.Join(dir, "backup")
	if err := os.WriteFile(stage, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	journal := stage + "\n" + backup + "\n" + Digest([]byte("new")) + "\n"
	if err := os.WriteFile(filepath.Join(dir, replacementJournal), []byte(journal), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Recover(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "old" {
		t.Fatalf("target = %q, %v", data, err)
	}
	for _, path := range []string{stage, backup, filepath.Join(dir, replacementJournal)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s was not cleaned up", path)
		}
	}
}

func TestRecoverPendingAcceptsCommittedTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, platform.ExecutableName("rc"))
	if err := os.WriteFile(target, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(dir, "backup")
	if err := os.WriteFile(backup, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(dir, "stage") + "\n" + backup + "\n" + Digest([]byte("new")) + "\n"
	if err := os.WriteFile(filepath.Join(dir, replacementJournal), []byte(journal), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Recover(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "new" {
		t.Fatalf("target = %q, %v", data, err)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatal("backup was not cleaned up")
	}
}
