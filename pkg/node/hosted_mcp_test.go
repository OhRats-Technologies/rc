package node

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

func TestVerifyApiControlAuthorityScopesExpiryAndRevocation(t *testing.T) {
	now := time.Now().UnixMilli()
	key := protocol.AuthorityApiKey{
		ID: "key_test", UserID: "user_test", PublicKey: "public-key",
		Scopes: []string{"execute", "manage-devices"},
	}
	snapshot := func(role string, keys ...protocol.AuthorityApiKey) string {
		value := protocol.AuthoritySnapshot{
			V: 1, WorkspaceID: "ws_test",
			Members: []protocol.AuthorityMember{{UserID: "user_test", Role: role}},
			ApiKeys: keys,
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	bootstrap := func(t *testing.T, value string) string {
		t.Helper()
		dir := t.TempDir()
		if err := BootstrapLock(dir, value, "https://rc.example.test"); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	dir := bootstrap(t, snapshot("owner", key))
	authority, err := VerifyApiControlAuthority(dir, "key_test")
	if err != nil {
		t.Fatal(err)
	}
	if authority.UserID != "user_test" || authority.Role != "owner" ||
		!authority.CanExecute || !authority.CanManageDevices {
		t.Fatalf("authority = %+v", authority)
	}

	expired := key
	expired.ExpiresAt = now - 1
	dir = bootstrap(t, snapshot("owner", expired))
	if _, err := VerifyApiControlAuthority(dir, "key_test"); !errors.Is(err, ErrLockApiKey) {
		t.Fatalf("expired key = %v", err)
	}

	dir = bootstrap(t, snapshot("owner"))
	if _, err := VerifyApiControlAuthority(dir, "key_test"); !errors.Is(err, ErrLockApiKey) {
		t.Fatalf("revoked key = %v", err)
	}

	dir = bootstrap(t, snapshot("viewer", key))
	if _, err := VerifyApiControlAuthority(dir, "key_test"); !errors.Is(err, ErrLockApiKey) {
		t.Fatalf("viewer key = %v", err)
	}

	readOnly := key
	readOnly.Scopes = []string{"read"}
	dir = bootstrap(t, snapshot("owner", readOnly))
	if _, err := VerifyApiControlAuthority(dir, "key_test"); !errors.Is(err, ErrLockApiKey) {
		t.Fatalf("read-only key = %v", err)
	}

	operator := key
	operator.Scopes = []string{"execute"}
	dir = bootstrap(t, snapshot("operator", operator))
	authority, err = VerifyApiControlAuthority(dir, "key_test")
	if err != nil || authority.CanManageDevices || !authority.CanExecute {
		t.Fatalf("operator authority = %+v, %v", authority, err)
	}
}

func TestScheduleAuthorityBindingsAndExpiry(t *testing.T) {
	now := time.Now().UnixMilli()
	grant := protocol.AuthorityScheduleGrant{
		ScheduleID: "schedule_test", DeviceID: "device_test", UserID: "user_test",
		SpecHash:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		MaxRuntimeMs: 60000, ExpiresAt: now + 60000,
	}
	snapshot := func(role string, value protocol.AuthorityScheduleGrant) string {
		encoded, err := json.Marshal(protocol.AuthoritySnapshot{
			V: 1, WorkspaceID: "ws_test",
			Members:        []protocol.AuthorityMember{{UserID: "user_test", Role: role}},
			ScheduleGrants: []protocol.AuthorityScheduleGrant{value},
		})
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	bootstrap := func(t *testing.T, value string) string {
		t.Helper()
		dir := t.TempDir()
		if err := BootstrapLock(dir, value, "https://rc.example.test"); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	dir := bootstrap(t, snapshot("owner", grant))
	got, err := ScheduleAuthority(dir, grant.ScheduleID, grant.DeviceID, grant.SpecHash)
	if err != nil || got.MaxRuntimeMs != grant.MaxRuntimeMs {
		t.Fatalf("schedule authority = %+v, %v", got, err)
	}
	if _, err := ScheduleAuthority(dir, grant.ScheduleID, "device_other", grant.SpecHash); !errors.Is(err, ErrLockScheduleGrant) {
		t.Fatalf("wrong device = %v", err)
	}
	if _, err := ScheduleAuthority(dir, grant.ScheduleID, grant.DeviceID, strings.Repeat("b", 64)); !errors.Is(err, ErrLockScheduleGrant) {
		t.Fatalf("wrong spec = %v", err)
	}

	expired := grant
	expired.ExpiresAt = now - 1
	dir = bootstrap(t, snapshot("owner", expired))
	if _, err := ScheduleAuthority(dir, grant.ScheduleID, grant.DeviceID, grant.SpecHash); !errors.Is(err, ErrLockScheduleGrant) {
		t.Fatalf("expired schedule = %v", err)
	}

	dir = bootstrap(t, snapshot("operator", grant))
	if _, err := ScheduleAuthority(dir, grant.ScheduleID, grant.DeviceID, grant.SpecHash); !errors.Is(err, ErrLockScheduleGrant) {
		t.Fatalf("operator schedule = %v", err)
	}
}
