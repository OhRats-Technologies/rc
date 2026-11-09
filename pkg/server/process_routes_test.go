package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

func TestBrowserProcessCreateListDetailAndPage(t *testing.T) {
	srv, ts := newAuthServer(t)
	user, err := srv.auth.CreateUser("Process Browser")
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.auth.EnsureWorkspaceMembership(defaultWorkspaceID, user.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	session, _, err := srv.auth.IssueSession(user.ID, "1h")
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: sessionCookieName, Value: session}
	deviceID := startMCPTestNode(t, srv, ts)
	waitTestNodeLock(t, srv, deviceID)

	body, _ := json.Marshal(map[string]any{"terminal": true})
	req, _ := http.NewRequest(http.MethodPost,
		ts.URL+"/api/v1/devices/"+deviceID+"/processes", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	res, payload := oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("process create = %d: %s", res.StatusCode, payload)
	}
	var created struct {
		ProcessID string `json:"processId"`
	}
	if err := json.Unmarshal(payload, &created); err != nil || created.ProcessID == "" {
		t.Fatalf("created process = %s", payload)
	}
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/v1/devices/"+deviceID+"/processes", nil)
	req.AddCookie(cookie)
	res, payload = oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK || !strings.Contains(string(payload), created.ProcessID) {
		t.Fatalf("process list = %d: %s", res.StatusCode, payload)
	}
	req, _ = http.NewRequest(http.MethodGet,
		ts.URL+"/devices/"+deviceID+"/processes/"+created.ProcessID, nil)
	req.AddCookie(cookie)
	res, payload = oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("process page = %d: %s", res.StatusCode, payload)
	}
	page := string(payload)
	if !strings.Contains(page, `data-process-page="`+created.ProcessID+`"`) ||
		!strings.Contains(page, `data-process-interactive="true"`) ||
		!strings.Contains(page, `id="terminal-host"`) ||
		!strings.Contains(page, `process-terminal`) {
		t.Fatalf("process page markup = %s", page)
	}
	srv.nodes.onProcessState(deviceID, protocol.NodeToServer{
		Type: protocol.TypeProcessState, ProcessID: created.ProcessID,
		Status: "exited", ExitCode: 0,
	})
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/v1/processes/"+created.ProcessID, nil)
	req.AddCookie(cookie)
	res, payload = oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK || !strings.Contains(string(payload), `"status":"exited"`) {
		t.Fatalf("process detail = %d: %s", res.StatusCode, payload)
	}
}
