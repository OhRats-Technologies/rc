package node

import (
	"strings"
	"testing"
)

func TestLockProblemExplainsOutOfSyncNode(t *testing.T) {
	if got := (&RemoteNodeStatus{}).LockProblem(); got != "" {
		t.Fatalf("status without RC Lock reported a problem: %q", got)
	}
	synced := &RemoteNodeStatus{RCLock: &RemoteRCLock{Generation: 3, Synced: true}}
	if got := synced.LockProblem(); got != "" {
		t.Fatalf("synced Node reported a problem: %q", got)
	}
	stale := &RemoteNodeStatus{RCLock: &RemoteRCLock{Generation: 2, Synced: false}}
	problem := stale.LockProblem()
	if !strings.Contains(problem, "generation 2") || !strings.Contains(problem, "Sync device access") {
		t.Fatalf("out-of-sync problem = %q", problem)
	}
}
