package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

func TestScheduleGrantRoutesAndAuthorityProjection(t *testing.T) {
	srv, ts := newAuthServer(t)
	user, err := srv.auth.CreateUser("Schedule Owner")
	if err != nil {
		t.Fatal(err)
	}
	control := seedTestControl(t, srv, user.ID, ts.URL)
	session, _, err := srv.auth.IssueSession(user.ID, "1h")
	if err != nil {
		t.Fatal(err)
	}
	device := authstore.Device{
		ID: "dev_schedule", WorkspaceID: defaultWorkspaceID, Name: "Schedule Node",
		Hostname: "host", Platform: "linux", Arch: "amd64", Version: "0.20.6",
		IdentityPublicKey: "identity-schedule", TransportPublicKey: "transport-schedule",
		CreatedAt: time.Now().UTC(),
	}
	if err := srv.auth.InsertDevice(device); err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: sessionCookieName, Value: session}
	body := map[string]any{
		"clientId": control.ID, "deviceId": device.ID,
		"specHash": strings.Repeat("a", 64), "maxRuntimeMs": 60000, "expiresAt": 0,
	}
	payload, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut,
		ts.URL+"/api/v1/workspaces/ws_default/schedule-grants/schedule_test", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	res, response := oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK || !strings.Contains(string(response), "lockHash") {
		t.Fatalf("schedule upsert = %d: %s", res.StatusCode, response)
	}
	grant, err := srv.auth.ScheduleGrant(defaultWorkspaceID, "schedule_test")
	if err != nil || grant.SpecHash != strings.Repeat("a", 64) {
		t.Fatalf("stored schedule = %+v, %v", grant, err)
	}
	snapshot, err := srv.authoritySnapshot(defaultWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snapshot, `"scheduleId":"schedule_test"`) {
		t.Fatalf("schedule missing from snapshot: %s", snapshot)
	}

	remove, _ := json.Marshal(map[string]string{"clientId": control.ID})
	req, _ = http.NewRequest(http.MethodDelete,
		ts.URL+"/api/v1/workspaces/ws_default/schedule-grants/schedule_test", bytes.NewReader(remove))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	res, response = oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("schedule delete = %d: %s", res.StatusCode, response)
	}
	if _, err := srv.auth.ScheduleGrant(defaultWorkspaceID, "schedule_test"); err == nil {
		t.Fatal("schedule remained after delete")
	}
}
