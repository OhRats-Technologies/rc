package node

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

func TestHostedMCPRejectsUnsignedGrantWhenLockExists(t *testing.T) {
	dir := t.TempDir()
	if err := BootstrapLock(dir, `{"v":1,"workspaceId":"ws_test"}`, "https://rc.example.test"); err != nil {
		t.Fatal(err)
	}
	hosted := NewHostedMCP(dir, &NodeState{}, nil)
	if _, err := hosted.verify(protocol.ServerToNode{}, "mcp:terminal"); err == nil {
		t.Fatal("invalid hosted permit must be rejected")
	}
}

func TestVerifyMcpGrantRequiresLockHashAndOwner(t *testing.T) {
	now := time.Now().UnixMilli()
	grantJSON := fmt.Sprintf(
		`{"v":2,"id":"mcp_test","userId":"user_test","clientId":"client_test",`+
			`"audience":"account","scopes":["mcp:terminal"],"issuedAt":%d}`,
		now,
	)
	control := &ControlAuthority{
		Grant: protocol.ControlGrant{UserID: "user_test", SigningPublicKey: "unused"},
		Role:  "owner",
	}

	missing := t.TempDir()
	if err := BootstrapLock(missing, `{"v":1,"workspaceId":"ws_test"}`, "https://rc.example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyMcpGrant(missing, grantJSON, "signature", control, "user_test", "dev_test"); !errors.Is(err, ErrLockMcpGrant) {
		t.Fatalf("missing lock grant error = %v", err)
	}

	present := t.TempDir()
	snapshot := fmt.Sprintf(
		`{"v":1,"workspaceId":"ws_test","mcpGrants":[{"id":"mcp_test","userId":"user_test","hash":"%s"}]}`,
		SnapshotHash(grantJSON),
	)
	if err := BootstrapLock(present, snapshot, "https://rc.example.test"); err != nil {
		t.Fatal(err)
	}
	demoted := *control
	demoted.Role = "operator"
	if _, err := VerifyMcpGrant(present, grantJSON, "signature", &demoted, "user_test", "dev_test"); !errors.Is(err, ErrLockMcpGrant) {
		t.Fatalf("demoted role error = %v", err)
	}
}
