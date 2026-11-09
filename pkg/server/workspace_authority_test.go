package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

func workspaceTestUser(t *testing.T, srv *Server, name string) *authstore.User {
	t.Helper()
	user, err := srv.auth.CreateUser(name)
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func workspaceTestCookie(t *testing.T, srv *Server, userID string) *http.Cookie {
	t.Helper()
	session, _, err := srv.auth.IssueSession(userID, "1h")
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: sessionCookieName, Value: session}
}

func workspaceStepUpCookie(t *testing.T, srv *Server, userID string) *http.Cookie {
	t.Helper()
	session, _, err := srv.auth.IssueSession(userID, "5m")
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: stepUpCookieName, Value: session}
}

func workspaceRequest(t *testing.T, tsURL, method, path, body string, cookies ...*http.Cookie) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, tsURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	return oauthDo(t, http.DefaultClient, req)
}

func TestWorkspaceMemberRolesRequireOwnerAndFreshPasskey(t *testing.T) {
	srv, ts := newAuthServer(t)
	owner := workspaceTestUser(t, srv, "Team Owner")
	member := workspaceTestUser(t, srv, "Team Member")
	outsider := workspaceTestUser(t, srv, "Team Outsider")
	if err := srv.auth.EnsureWorkspace("ws_team", "Team"); err != nil {
		t.Fatal(err)
	}
	if err := srv.auth.EnsureWorkspaceMembership("ws_team", owner.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	if err := srv.auth.EnsureWorkspaceMembership("ws_team", member.ID, "viewer"); err != nil {
		t.Fatal(err)
	}
	ownerCookie, memberCookie := workspaceTestCookie(t, srv, owner.ID), workspaceTestCookie(t, srv, member.ID)
	outsiderCookie := workspaceTestCookie(t, srv, outsider.ID)

	path := "/api/v1/workspaces/ws_team/members/" + member.ID
	res, payload := workspaceRequest(t, ts.URL, http.MethodPatch, path, `{"role":"operator"}`, ownerCookie)
	if res.StatusCode != http.StatusUnauthorized || !strings.Contains(string(payload), "fresh passkey") {
		t.Fatalf("role change without step-up = %d: %s", res.StatusCode, payload)
	}
	res, payload = workspaceRequest(t, ts.URL, http.MethodPatch, path, `{"role":"operator"}`,
		memberCookie, workspaceStepUpCookie(t, srv, member.ID))
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer role change = %d: %s", res.StatusCode, payload)
	}
	res, payload = workspaceRequest(t, ts.URL, http.MethodPatch, path, `{"role":"operator"}`,
		outsiderCookie, workspaceStepUpCookie(t, srv, outsider.ID))
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("outsider role change = %d: %s", res.StatusCode, payload)
	}
	res, payload = workspaceRequest(t, ts.URL, http.MethodPatch, path, `{"role":"operator"}`,
		ownerCookie, workspaceStepUpCookie(t, srv, owner.ID))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("owner role change = %d: %s", res.StatusCode, payload)
	}
	role, err := srv.auth.WorkspaceRole(member.ID, "ws_team")
	if err != nil || role != "operator" {
		t.Fatalf("member role = %q, %v", role, err)
	}
	res, payload = workspaceRequest(t, ts.URL, http.MethodPatch,
		"/api/v1/workspaces/ws_team/members/"+owner.ID, `{"role":"viewer"}`,
		ownerCookie, workspaceStepUpCookie(t, srv, owner.ID))
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("last owner demotion = %d: %s", res.StatusCode, payload)
	}
	res, payload = workspaceRequest(t, ts.URL, http.MethodDelete, path, "",
		ownerCookie, workspaceStepUpCookie(t, srv, owner.ID))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("member removal = %d: %s", res.StatusCode, payload)
	}
	if _, err := srv.auth.WorkspaceRole(member.ID, "ws_team"); err == nil {
		t.Fatal("removed member still has a role")
	}
}

func TestWorkspaceAccessAndActivityPages(t *testing.T) {
	srv, ts := newAuthServer(t)
	owner := workspaceTestUser(t, srv, "Access Owner")
	member := workspaceTestUser(t, srv, "Access Member")
	if err := srv.auth.EnsureWorkspace("ws_access", "Access Workspace"); err != nil {
		t.Fatal(err)
	}
	if err := srv.auth.EnsureWorkspaceMembership("ws_access", owner.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	if err := srv.auth.EnsureWorkspaceMembership("ws_access", member.ID, "operator"); err != nil {
		t.Fatal(err)
	}
	ownerCookie := workspaceTestCookie(t, srv, owner.ID)
	memberCookie := workspaceTestCookie(t, srv, member.ID)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/workspaces/ws_access/access", nil)
	req.AddCookie(ownerCookie)
	res, payload := oauthDo(t, http.DefaultClient, req)
	page := string(payload)
	if res.StatusCode != http.StatusOK || !strings.Contains(page, `data-authority-workspace="ws_access"`) ||
		!strings.Contains(page, "Access Member") || !strings.Contains(page, "role-form") ||
		!strings.Contains(page, "data-authority-sync") || !strings.Contains(page, "asset") {
		t.Fatalf("access page = %d: %s", res.StatusCode, page)
	}
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/workspaces/ws_access/access", nil)
	req.AddCookie(memberCookie)
	res, _ = oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("operator access page = %d, want 404", res.StatusCode)
	}

	srv.publishAudit("ws_access", owner.ID, "", "workspace.member.role",
		map[string]any{"memberId": member.ID, "name": member.Name, "role": "operator"})
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/workspaces/ws_access/activity", nil)
	req.AddCookie(memberCookie)
	res, payload = oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK || !strings.Contains(string(payload), `data-activity-page="ws_access"`) ||
		!strings.Contains(string(payload), "WORKSPACE.MEMBER.ROLE") {
		t.Fatalf("activity page = %d: %s", res.StatusCode, payload)
	}
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/v1/workspaces/ws_access/activity", nil)
	req.AddCookie(memberCookie)
	res, payload = oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK || !strings.Contains(string(payload), "workspace.member.role") {
		t.Fatalf("activity API = %d: %s", res.StatusCode, payload)
	}
}

func TestWorkspaceInviteJoinAndAccessJSON(t *testing.T) {
	srv, ts := newAuthServer(t)
	owner := workspaceTestUser(t, srv, "Invite Owner")
	joiner := workspaceTestUser(t, srv, "Invite Joiner")
	if err := srv.auth.EnsureWorkspace("ws_invite", "Invite Workspace"); err != nil {
		t.Fatal(err)
	}
	if err := srv.auth.EnsureWorkspaceMembership("ws_invite", owner.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	ownerCookie := workspaceTestCookie(t, srv, owner.ID)
	joinerCookie := workspaceTestCookie(t, srv, joiner.ID)

	body, _ := json.Marshal(map[string]string{"role": "viewer"})
	res, payload := workspaceRequest(t, ts.URL, http.MethodPost,
		"/api/v1/workspaces/ws_invite/invites", string(body), ownerCookie)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("invite create = %d: %s", res.StatusCode, payload)
	}
	var created struct {
		Token string `json:"token"`
		URL   string `json:"url"`
	}
	if err := json.Unmarshal(payload, &created); err != nil || created.Token == "" ||
		!strings.Contains(created.URL, "/join?invite=") {
		t.Fatalf("created invite = %s", payload)
	}
	if res, payload := workspaceRequest(t, ts.URL, http.MethodGet,
		"/join?invite="+created.Token, "", joinerCookie); res.StatusCode != http.StatusOK ||
		!strings.Contains(string(payload), "Join workspace") {
		t.Fatalf("join page = %d: %s", res.StatusCode, payload)
	}
	joinBody, _ := json.Marshal(map[string]string{"token": created.Token})
	res, payload = workspaceRequest(t, ts.URL, http.MethodPost,
		"/api/v1/workspaces/join", string(joinBody), joinerCookie)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("join = %d: %s", res.StatusCode, payload)
	}
	role, err := srv.auth.WorkspaceRole(joiner.ID, "ws_invite")
	if err != nil || role != "viewer" {
		t.Fatalf("joined role = %q, %v", role, err)
	}
	res, payload = workspaceRequest(t, ts.URL, http.MethodPost,
		"/api/v1/workspaces/join", string(joinBody), joinerCookie)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("replayed join = %d: %s", res.StatusCode, payload)
	}

	res, payload = workspaceRequest(t, ts.URL, http.MethodGet,
		"/api/v1/workspaces/ws_invite/access", "", ownerCookie)
	if res.StatusCode != http.StatusOK || !bytes.Contains(payload, []byte(`"members"`)) ||
		!bytes.Contains(payload, []byte(joiner.Name)) {
		t.Fatalf("access JSON = %d: %s", res.StatusCode, payload)
	}
}

func TestWorkspaceLifecycleAndSidebarLinks(t *testing.T) {
	srv, ts := newAuthServer(t)
	owner := workspaceTestUser(t, srv, "Lifecycle Owner")
	member := workspaceTestUser(t, srv, "Lifecycle Member")
	cookie := workspaceTestCookie(t, srv, owner.ID)

	body, _ := json.Marshal(map[string]string{"name": "Lifecycle Workspace"})
	res, payload := workspaceRequest(t, ts.URL, http.MethodPost, "/api/v1/workspaces", string(body), cookie)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("workspace create = %d: %s", res.StatusCode, payload)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload, &created); err != nil || created.ID == "" {
		t.Fatalf("created workspace = %s", payload)
	}
	res, payload = workspaceRequest(t, ts.URL, http.MethodGet, "/api/v1/workspaces", "", cookie)
	if res.StatusCode != http.StatusOK || !bytes.Contains(payload, []byte(created.ID)) {
		t.Fatalf("workspace list = %d: %s", res.StatusCode, payload)
	}
	res, payload = workspaceRequest(t, ts.URL, http.MethodPatch,
		"/api/v1/workspaces/"+created.ID, `{"name":"Renamed Workspace"}`, cookie)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("workspace rename = %d: %s", res.StatusCode, payload)
	}
	if workspace, err := srv.auth.WorkspaceByID(created.ID); err != nil || workspace.Name != "Renamed Workspace" {
		t.Fatalf("renamed workspace = %+v, %v", workspace, err)
	}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/devices", nil)
	req.AddCookie(cookie)
	res, payload = oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK || !strings.Contains(string(payload), "Renamed Workspace") ||
		!strings.Contains(string(payload), "Manage access") || !strings.Contains(string(payload), "Activity") {
		t.Fatalf("sidebar links = %d: %s", res.StatusCode, payload)
	}
	if err := srv.auth.EnsureWorkspaceMembership(created.ID, member.ID, "operator"); err != nil {
		t.Fatal(err)
	}
	res, payload = workspaceRequest(t, ts.URL, http.MethodPost,
		"/api/v1/workspaces/"+created.ID+"/leave", "", workspaceTestCookie(t, srv, member.ID))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("workspace leave = %d: %s", res.StatusCode, payload)
	}
	if _, err := srv.auth.WorkspaceRole(member.ID, created.ID); err == nil {
		t.Fatal("left member still has a role")
	}
	res, payload = workspaceRequest(t, ts.URL, http.MethodDelete, "/api/v1/workspaces/"+created.ID, "", cookie)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("workspace delete = %d: %s", res.StatusCode, payload)
	}
	if _, err := srv.auth.WorkspaceByID(created.ID); err == nil {
		t.Fatal("deleted workspace still exists")
	}
}
