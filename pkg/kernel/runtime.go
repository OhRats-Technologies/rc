package kernel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type ComponentEntry struct {
	Current *LoadedComponent
	Active  bool
	Error   string
}

type Runtime struct {
	mu          sync.Mutex
	dir         string
	engine      *Engine
	changes     *ChangeTracker
	entries     map[string]*ComponentEntry
	failedPaths map[string]string
}

func NewRuntime(dir string) (*Runtime, error) {
	eng, err := NewEngine()
	if err != nil {
		return nil, err
	}
	return &Runtime{
		dir:         filepath.Clean(dir),
		engine:      eng,
		changes:     NewChangeTracker(),
		entries:     make(map[string]*ComponentEntry),
		failedPaths: make(map[string]string),
	}, nil
}

func (r *Runtime) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.engine != nil {
		r.engine.Close()
		r.engine = nil
	}
}

func (r *Runtime) Reconcile() (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entries, err := os.ReadDir(r.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	confirmed := make(map[string]bool)
	changed := false

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".wasm") {
			continue
		}
		path := filepath.Join(r.dir, entry.Name())
		confirmed[path] = true

		ch := r.loadPath(path)
		if ch {
			changed = true
		}
	}

	r.changes.Retain(confirmed)
	return changed, nil
}

func (r *Runtime) loadPath(path string) bool {
	check := r.changes.Check(path)
	if check.Unchanged {
		return false
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		r.changes.Forget(path)
		r.failedPaths[path] = err.Error()
		return true
	}

	comp, err := LoadComponentBytes(path, bytes)
	if err != nil {
		r.changes.Record(path, check.Fingerprint)
		r.failedPaths[path] = err.Error()
		return true
	}

	delete(r.failedPaths, path)
	r.changes.Record(path, check.Fingerprint)

	id := comp.Descriptor.ID
	entry, exists := r.entries[id]
	if !exists {
		r.entries[id] = &ComponentEntry{
			Current: comp,
			Active:  true,
		}
		return true
	}

	if entry.Current.Digest != comp.Digest {
		entry.Current = comp
		entry.Active = true
		entry.Error = ""
		return true
	}
	return false
}

// Count reports the number of loaded component identities.
func (r *Runtime) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

func (r *Runtime) Entry(id string) (*ComponentEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[id]
	return e, ok
}

func (r *Runtime) SkippedCount() int {
	return r.changes.Skipped
}

func (r *Runtime) FailedPaths() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string)
	for k, v := range r.failedPaths {
		out[k] = v
	}
	return out
}

func (r *Runtime) Summary() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return fmt.Sprintf("Runtime(dir=%s, loaded=%d, failed=%d)", r.dir, len(r.entries), len(r.failedPaths))
}
