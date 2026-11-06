package platform

import (
	"errors"
	"testing"
)

func TestRunLockIsExclusiveAndReusable(t *testing.T) {
	temp := t.TempDir()

	first, err := AcquireRunLock(temp)
	if err != nil {
		t.Fatalf("first AcquireRunLock failed: %v", err)
	}

	second, err := AcquireRunLock(temp)
	if err == nil {
		second.Close()
		t.Fatal("expected second AcquireRunLock to fail with already running")
	}
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("expected ErrAlreadyRunning, got: %v", err)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("first.Close failed: %v", err)
	}

	third, err := AcquireRunLock(temp)
	if err != nil {
		t.Fatalf("third AcquireRunLock failed after release: %v", err)
	}
	third.Close()
}
