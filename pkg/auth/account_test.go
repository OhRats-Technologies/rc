package auth

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

func TestDeleteAccountRemovesOwnedWorkspacesAndPreservesAttribution(t *testing.T) {
	store := openStore(t)
	owner, err := store.CreateUser("Delete Owner")
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateUser("Other Owner")
	if err != nil {
		t.Fatal(err)
	}
	third, err := store.CreateUser("Third Owner")
	if err != nil {
		t.Fatal(err)
	}
	owned, err := store.CreateWorkspace("Owned", owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := store.CreateWorkspace("Shared", owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetWorkspaceMembership(shared.ID, other.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateWorkspaceMemberRole(shared.ID, owner.ID, "operator"); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureWorkspaceMembership(shared.ID, third.ID, "viewer"); err != nil {
		t.Fatal(err)
	}
	device := Device{
		ID: "dev_delete_owned", WorkspaceID: owned.ID, Name: "Node",
		Hostname: "host", Platform: "linux", Arch: "amd64", Version: "0.20.6",
		IdentityPublicKey: "identity-delete", TransportPublicKey: "transport-delete",
		CreatedAt: time.Now().UTC(),
	}
	if err := store.InsertDevice(device); err != nil {
		t.Fatal(err)
	}
	inviteID, _, err := store.CreateWorkspaceInvite(shared.ID, owner.ID, "viewer", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordAuditEvent(AuditEvent{
		WorkspaceID: shared.ID, UserID: owner.ID, Kind: "workspace.invite.created",
		Detail: `{"inviteId":"` + inviteID + `"}`,
	}); err != nil {
		t.Fatal(err)
	}

	devices, err := store.DeleteAccount(owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0] != device.ID {
		t.Fatalf("revoked devices = %v", devices)
	}
	if _, err := store.WorkspaceByID(owned.ID); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("owned workspace survived: %v", err)
	}
	if revoked, err := store.DeviceRevoked(device.ID); err != nil || !revoked {
		t.Fatalf("device tombstone = %v, %v", revoked, err)
	}
	if _, err := store.WorkspaceRole(owner.ID, shared.ID); err == nil {
		t.Fatal("deleted account still has membership")
	}
	if role, err := store.WorkspaceRole(other.ID, shared.ID); err != nil || role != "owner" {
		t.Fatalf("surviving owner = %q, %v", role, err)
	}
	workspace, err := store.WorkspaceByID(shared.ID)
	if err != nil || workspace.CreatedBy != DeletedAccountID {
		t.Fatalf("workspace attribution = %+v, %v", workspace, err)
	}
	invites, err := store.ListWorkspaceInvites(shared.ID)
	if err != nil || len(invites) != 1 || invites[0].CreatedBy != DeletedAccountID {
		t.Fatalf("invite attribution = %+v, %v", invites, err)
	}
	events, err := store.WorkspaceAuditEvents(shared.ID, 10)
	if err != nil || len(events) != 1 || events[0].UserID != DeletedAccountID {
		t.Fatalf("event attribution = %+v, %v", events, err)
	}
	if _, err := store.GetUser(owner.ID); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("deleted user lookup = %v", err)
	}
	if count, err := store.UserCount(); err != nil || count != 2 {
		t.Fatalf("active users = %d, %v", count, err)
	}
	if deleted, err := store.GetUser(DeletedAccountID); err != nil || deleted.Name != DeletedAccountName {
		t.Fatalf("deleted account principal = %+v, %v", deleted, err)
	}
}

func TestDeleteAccountReturnsToZeroActiveUsers(t *testing.T) {
	store := openStore(t)
	user, err := store.CreateUser("Only Human")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeleteAccount(user.ID); err != nil {
		t.Fatal(err)
	}
	if count, err := store.UserCount(); err != nil || count != 0 {
		t.Fatalf("active users = %d, %v", count, err)
	}
	users, err := store.ListUsers()
	if err != nil || len(users) != 0 {
		t.Fatalf("listed users = %+v, %v", users, err)
	}
	if _, err := store.GetUser(DeletedAccountID); err != nil {
		t.Fatalf("deleted account principal missing: %v", err)
	}
	if _, err := store.DeleteAccount(DeletedAccountID); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("deleting the tombstone = %v", err)
	}
}

func TestDeleteAccountRemovesOAuthCredentials(t *testing.T) {
	store := openStore(t)
	user, err := store.CreateUser("OAuth Owner")
	if err != nil {
		t.Fatal(err)
	}
	clientID, err := store.RegisterOAuthClient("OAuth Client", []string{"https://client.example/callback"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateOAuthRequest(clientID, user.ID, "https://client.example/callback",
		"state", base64.RawURLEncoding.EncodeToString(make([]byte, 32)), "https://rc.example.test/mcp",
		[]string{"mcp:observe"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.IssueOAuthCode(clientID, user.ID, "https://client.example/callback",
		base64.RawURLEncoding.EncodeToString(make([]byte, 32)), "https://rc.example.test/mcp",
		"mcp:observe", "grant_delete"); err != nil {
		t.Fatal(err)
	}
	access, _, _, _, err := store.IssueOAuthTokens(clientID, user.ID,
		"https://rc.example.test/mcp", "mcp:observe", "grant_delete")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeleteAccount(user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LookupOAuthAccess(access, "https://rc.example.test/mcp"); !errors.Is(err, ErrOAuthTokenNotFound) {
		t.Fatalf("deleted account OAuth access = %v", err)
	}
	for _, table := range []string{"oauth_codes", "oauth_requests", "oauth_tokens"} {
		var count int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE user_id = ?;`, user.ID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s retained %d rows for a deleted account", table, count)
		}
	}
}
