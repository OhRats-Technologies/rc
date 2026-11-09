package auth

import (
	"errors"
	"testing"
	"time"
)

func TestWorkspaceLifecycleAndMemberRoles(t *testing.T) {
	store := openStore(t)
	owner, err := store.CreateUser("Owner")
	if err != nil {
		t.Fatal(err)
	}
	member, err := store.CreateUser("Member")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := store.CreateWorkspace("Team", owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureWorkspaceMembership(workspace.ID, member.ID, "viewer"); err != nil {
		t.Fatal(err)
	}
	previous, err := store.UpdateWorkspaceMemberRole(workspace.ID, member.ID, "operator")
	if err != nil || previous != "viewer" {
		t.Fatalf("role update = %q, %v", previous, err)
	}
	if _, err := store.UpdateWorkspaceMemberRole(workspace.ID, owner.ID, "viewer"); !errors.Is(err, ErrLastWorkspaceOwner) {
		t.Fatalf("last owner demotion = %v", err)
	}
	if _, err := store.LeaveWorkspace(workspace.ID, owner.ID); !errors.Is(err, ErrLastWorkspaceOwner) {
		t.Fatalf("last owner leave = %v", err)
	}
	if role, err := store.LeaveWorkspace(workspace.ID, member.ID); err != nil || role != "operator" {
		t.Fatalf("member leave = %q, %v", role, err)
	}
	if err := store.RenameWorkspace(workspace.ID, "Renamed"); err != nil {
		t.Fatal(err)
	}
	if renamed, err := store.WorkspaceByID(workspace.ID); err != nil || renamed.Name != "Renamed" {
		t.Fatalf("renamed workspace = %+v, %v", renamed, err)
	}
	if err := store.DeleteWorkspace(workspace.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.WorkspaceByID(workspace.ID); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("deleted workspace = %v", err)
	}
}

func TestWorkspaceInvitesAndAuditEvents(t *testing.T) {
	store := openStore(t)
	owner, err := store.CreateUser("Invite Owner")
	if err != nil {
		t.Fatal(err)
	}
	joiner, err := store.CreateUser("Invite Joiner")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := store.CreateWorkspace("Invite Team", owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	id, token, err := store.CreateWorkspaceInvite(workspace.ID, owner.ID, "viewer", time.Now().Add(time.Hour))
	if err != nil || id == "" || token == "" {
		t.Fatalf("invite = %q, %q, %v", id, token, err)
	}
	if !store.WorkspaceInviteValid(token) {
		t.Fatal("new invite is invalid")
	}
	joined, role, err := store.ConsumeWorkspaceInvite(token, joiner.ID)
	if err != nil || joined != workspace.ID || role != "viewer" {
		t.Fatalf("consume = %q, %q, %v", joined, role, err)
	}
	if store.WorkspaceInviteValid(token) {
		t.Fatal("consumed invite is still valid")
	}
	if _, _, err := store.ConsumeWorkspaceInvite(token, joiner.ID); !errors.Is(err, ErrWorkspaceInviteNotFound) {
		t.Fatalf("replayed invite = %v", err)
	}
	if err := store.RecordAuditEvent(AuditEvent{
		WorkspaceID: workspace.ID, UserID: owner.ID,
		Kind: "workspace.member.joined", Detail: `{"role":"viewer"}`,
	}); err != nil {
		t.Fatal(err)
	}
	events, err := store.WorkspaceAuditEvents(workspace.ID, 10)
	if err != nil || len(events) != 1 || events[0].Kind != "workspace.member.joined" {
		t.Fatalf("audit events = %+v, %v", events, err)
	}
}
