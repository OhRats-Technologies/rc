package kernel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnchangedComponentIsNotReloaded(t *testing.T) {
	temp := t.TempDir()
	compDir := filepath.Join(temp, "components")
	if err := os.MkdirAll(compDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	path := filepath.Join(compDir, "provider.wasm")
	if err := os.WriteFile(path, []byte("not a WebAssembly component"), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	rt, err := NewRuntime(compDir)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	changed, err := rt.Reconcile()
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true on initial scan")
	}
	if _, ok := rt.FailedPaths()[path]; !ok {
		t.Fatalf("expected failedPaths to contain %s", path)
	}
	if rt.SkippedCount() != 0 {
		t.Fatalf("expected 0 skipped, got %d", rt.SkippedCount())
	}

	// Second reconcile without file changes must skip reading
	changed, err = rt.Reconcile()
	if err != nil {
		t.Fatalf("Reconcile 2 failed: %v", err)
	}
	if changed {
		t.Fatal("expected changed=false on unchanged reconcile")
	}
	if rt.SkippedCount() != 1 {
		t.Fatalf("expected 1 skipped, got %d", rt.SkippedCount())
	}

	// Update file content
	if err := os.WriteFile(path, []byte("still not a WebAssembly component"), 0644); err != nil {
		t.Fatalf("rewrite file failed: %v", err)
	}

	_, err = rt.Reconcile()
	if err != nil {
		t.Fatalf("Reconcile 3 failed: %v", err)
	}
	if rt.SkippedCount() != 1 {
		t.Fatalf("expected skipped to still be 1 after modification, got %d", rt.SkippedCount())
	}

	// Subsequent reconcile should skip again
	changed, err = rt.Reconcile()
	if err != nil {
		t.Fatalf("Reconcile 4 failed: %v", err)
	}
	if changed {
		t.Fatal("expected changed=false")
	}
	if rt.SkippedCount() != 2 {
		t.Fatalf("expected 2 skipped, got %d", rt.SkippedCount())
	}
}

func TestLoadGoComponentWasm(t *testing.T) {
	wasmPath := filepath.Join("..", "..", "dist", "components", "authority-store.wasm")
	data, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Skip("authority-store.wasm not yet built, skipping")
	}

	temp := t.TempDir()
	compDir := filepath.Join(temp, "components")
	_ = os.MkdirAll(compDir, 0755)

	target := filepath.Join(compDir, "authority-store.wasm")
	if err := os.WriteFile(target, data, 0644); err != nil {
		t.Fatalf("copy wasm failed: %v", err)
	}

	rt, err := NewRuntime(compDir)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	changed, err := rt.Reconcile()
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true on component load")
	}

	entry, ok := rt.Entry("authority-store")
	if !ok {
		t.Fatal("expected authority-store entry in runtime")
	}
	if !entry.Active {
		t.Fatal("expected authority-store to be active")
	}
	if len(entry.Current.Digest) != 64 {
		t.Fatalf("expected 64-char hex digest, got %s", entry.Current.Digest)
	}
}
