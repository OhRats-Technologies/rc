package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OhRats-Technologies/rc/pkg/kernel"
)

func profileComponents(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read profile %s: %v", path, err)
	}
	text := string(data)
	start := strings.Index(text, "components = [")
	if start < 0 {
		t.Fatalf("%s has no components list", path)
	}
	rest := text[start+len("components = ["):]
	end := strings.Index(rest, "]")
	if end < 0 {
		t.Fatalf("%s has an unterminated components list", path)
	}
	var names []string
	for _, line := range strings.Split(rest[:end], "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), ","))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.Trim(line, `"'`)
		if line != "" {
			names = append(names, line)
		}
	}
	if len(names) == 0 {
		t.Fatalf("%s has an empty components list", path)
	}
	return names
}

// TestCoreProfileComponentsLoad builds every member of the core profile and
// verifies the kernel runtime discovers each artifact with a stable digest.
func TestCoreProfileComponentsLoad(t *testing.T) {
	root := repoRoot(t)
	members := profileComponents(t, filepath.Join(root, "profiles", "core.toml"))
	components := componentArtifacts(t, members...)

	rt, err := kernel.NewRuntime(components)
	if err != nil {
		t.Fatalf("kernel runtime: %v", err)
	}
	defer rt.Close()

	changed, err := rt.Reconcile()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !changed {
		t.Fatal("first reconcile must report a change")
	}
	if failed := rt.FailedPaths(); len(failed) != 0 {
		t.Fatalf("components failed to load: %v", failed)
	}
	for _, name := range members {
		entry, ok := rt.Entry(name)
		if !ok {
			t.Fatalf("runtime did not load %s", name)
		}
		if !entry.Active {
			t.Fatalf("%s is not active: %s", name, entry.Error)
		}
		if len(entry.Current.Digest) != 64 {
			t.Fatalf("%s digest %q is not sha256 hex", name, entry.Current.Digest)
		}
		if len(entry.Current.WasmBytes) < 8 || string(entry.Current.WasmBytes[:4]) != "\x00asm" {
			t.Fatalf("%s is not a WebAssembly component", name)
		}
	}

	changed, err = rt.Reconcile()
	if err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if changed {
		t.Fatal("unchanged components must not reload")
	}
	if rt.SkippedCount() != len(members) {
		t.Fatalf("skipped = %d, want %d", rt.SkippedCount(), len(members))
	}
}

func TestComponentReloadAndFailureIsolation(t *testing.T) {
	components := componentArtifacts(t, "artifact-cache-local", "storage-fixture")

	rt, err := kernel.NewRuntime(components)
	if err != nil {
		t.Fatalf("kernel runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Reconcile(); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	first, ok := rt.Entry("artifact-cache-local")
	if !ok {
		t.Fatal("artifact-cache-local missing")
	}
	firstDigest := first.Current.Digest

	original, err := os.ReadFile(filepath.Join(components, "artifact-cache-local.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	updated := append(append([]byte{}, original...), 0x00)
	if err := os.WriteFile(filepath.Join(components, "artifact-cache-local.wasm"), updated, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Reconcile(); err != nil {
		t.Fatalf("reconcile after update: %v", err)
	}
	reloaded, ok := rt.Entry("artifact-cache-local")
	if !ok {
		t.Fatal("artifact-cache-local missing after update")
	}
	if reloaded.Current.Digest == firstDigest {
		t.Fatal("updated component digest did not change")
	}

	// A malformed artifact must be isolated; healthy components stay active.
	if err := os.WriteFile(filepath.Join(components, "broken.wasm"), []byte("not wasm"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Reconcile(); err != nil {
		t.Fatalf("reconcile with broken component: %v", err)
	}
	if _, ok := rt.FailedPaths()[filepath.Join(components, "broken.wasm")]; !ok {
		t.Fatalf("broken.wasm was not recorded as failed: %v", rt.FailedPaths())
	}
	if _, ok := rt.Entry("storage-fixture"); !ok {
		t.Fatal("healthy component was dropped after a neighbouring failure")
	}
	if !strings.Contains(rt.Summary(), "loaded=2") {
		t.Fatalf("unexpected runtime summary: %s", rt.Summary())
	}
}
