//go:build darwin || linux

package main

import (
	"strings"
	"testing"
)

func TestRunLockRejectsSecondNode(t *testing.T) {
	dir := t.TempDir()
	first, err := acquireRunLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()

	second, err := acquireRunLock(dir)
	if second != nil {
		second.Close()
		t.Fatal("second RC Node acquired the same enrollment lock")
	}
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("unexpected contention error: %v", err)
	}
}
