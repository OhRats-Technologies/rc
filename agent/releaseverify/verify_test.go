package releaseverify

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func releaseFiles(t *testing.T) ([]byte, []byte) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	releaseDir := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "release"))
	manifest, err := os.ReadFile(filepath.Join(releaseDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	signature, err := os.ReadFile(filepath.Join(releaseDir, "manifest.sig"))
	if err != nil {
		t.Fatal(err)
	}
	return manifest, signature
}

func TestCommittedReleaseSignature(t *testing.T) {
	manifestBytes, signatureBytes := releaseFiles(t)
	manifest, err := Verify(manifestBytes, signatureBytes)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "0.10.0" {
		t.Fatalf("unexpected version %s", manifest.Version)
	}
	if len(manifest.Artifacts) != 4 {
		t.Fatalf("unexpected artifact count %d", len(manifest.Artifacts))
	}
}

func TestTamperedManifestRejected(t *testing.T) {
	manifestBytes, signatureBytes := releaseFiles(t)
	manifestBytes = append([]byte(nil), manifestBytes...)
	manifestBytes[len(manifestBytes)-2] ^= 1
	if _, err := Verify(manifestBytes, signatureBytes); err == nil {
		t.Fatal("tampered manifest was accepted")
	}
}

func TestVersionComparisonRejectsRollback(t *testing.T) {
	comparison, err := CompareVersions("0.8.1", "0.8.2")
	if err != nil {
		t.Fatal(err)
	}
	if comparison >= 0 {
		t.Fatal("older release did not compare lower")
	}
}
