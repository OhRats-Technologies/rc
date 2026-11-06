package kernel

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Fingerprint struct {
	Length   int64
	Modified time.Time
}

func ReadFingerprint(path string) (*Fingerprint, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	return &Fingerprint{
		Length:   fi.Size(),
		Modified: fi.ModTime(),
	}, true
}

func (f *Fingerprint) Equal(other *Fingerprint) bool {
	if f == nil || other == nil {
		return f == other
	}
	return f.Length == other.Length && f.Modified.Equal(other.Modified)
}

type CheckResult struct {
	Unchanged   bool
	Fingerprint *Fingerprint
}

type ChangeTracker struct {
	mu      sync.Mutex
	entries map[string]*Fingerprint
	Skipped int
}

func NewChangeTracker() *ChangeTracker {
	return &ChangeTracker{
		entries: make(map[string]*Fingerprint),
	}
}

func (c *ChangeTracker) Check(path string) CheckResult {
	clean := filepath.Clean(path)
	c.mu.Lock()
	defer c.mu.Unlock()

	fp, ok := ReadFingerprint(clean)
	if !ok {
		return CheckResult{Unchanged: false, Fingerprint: nil}
	}

	prev, exists := c.entries[clean]
	unchanged := exists && prev.Equal(fp)
	if unchanged {
		c.Skipped++
	}
	return CheckResult{
		Unchanged:   unchanged,
		Fingerprint: fp,
	}
}

func (c *ChangeTracker) Record(path string, fp *Fingerprint) {
	clean := filepath.Clean(path)
	c.mu.Lock()
	defer c.mu.Unlock()
	if fp != nil {
		c.entries[clean] = fp
	} else {
		delete(c.entries, clean)
	}
}

func (c *ChangeTracker) Forget(path string) {
	clean := filepath.Clean(path)
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, clean)
}

func (c *ChangeTracker) Retain(paths map[string]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for p := range c.entries {
		if !paths[p] {
			delete(c.entries, p)
		}
	}
}
