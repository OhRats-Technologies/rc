package executionruntime

import (
	"testing"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
)

func attachedLeases(now uint64) *Leases {
	return NewLeases(now, KindAttached, 60_000, 350, witTypes.None[uint64]())
}

func TestAttachedExecutionExpiresWithoutInitialAttachment(t *testing.T) {
	leases := attachedLeases(1_000)
	if leases.Expired(60_999) {
		t.Fatal("expired before the reattach grace elapsed")
	}
	if !leases.Expired(61_000) {
		t.Fatal("did not expire at the reattach deadline")
	}
}

func TestReattachCancelsDeadlineAndIgnoresStaleWriter(t *testing.T) {
	leases := attachedLeases(1_000)
	if err := leases.Attach("old"); err != nil {
		t.Fatalf("attach: %v", err)
	}
	leases.Detach("old", 2_000)
	if err := leases.Attach("new"); err != nil {
		t.Fatalf("reattach: %v", err)
	}
	leases.Detach("old", 70_000)
	if leases.Expired(1_000_000) {
		t.Fatal("stale detach restarted the reattach grace")
	}
	leases.Detach("new", 1_000_000)
	if leases.Expired(1_059_999) {
		t.Fatal("expired before the current controller's grace elapsed")
	}
	if !leases.Expired(1_060_000) {
		t.Fatal("did not expire after the current controller detached")
	}
}

func TestManagedExecutionIgnoresAttachmentButObeysMaxRuntime(t *testing.T) {
	leases := NewLeases(5_000, KindManaged, 60_000, 350, witTypes.Some[uint64](10_000))
	if err := leases.Attach("writer"); err == nil {
		t.Fatal("managed execution accepted an attachment")
	}
	leases.Detach("writer", 6_000)
	if leases.Expired(14_999) {
		t.Fatal("expired before the maximum runtime elapsed")
	}
	if !leases.Expired(15_000) {
		t.Fatal("did not expire at the maximum runtime")
	}
}

func TestTerminateEscalatesAfterItsGrace(t *testing.T) {
	leases := NewLeases(0, KindScheduled, 60_000, 350, witTypes.None[uint64]())
	leases.Terminate(10_000)
	if leases.Expired(10_349) {
		t.Fatal("expired before the terminate grace elapsed")
	}
	if !leases.Expired(10_350) {
		t.Fatal("did not expire after the terminate grace")
	}
}

func TestScheduledExecutionObeysMaxRuntime(t *testing.T) {
	leases := NewLeases(7_000, KindScheduled, 60_000, 350, witTypes.Some[uint64](2_500))
	if leases.Expired(9_499) {
		t.Fatal("expired before the maximum runtime elapsed")
	}
	if !leases.Expired(9_500) {
		t.Fatal("did not expire at the maximum runtime")
	}
}
