package server

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/crypto"
	"golang.org/x/net/websocket"
)

func TestLockTransitionInvalidatesLiveControlTransport(t *testing.T) {
	srv, ts := newAuthServer(t)
	user, err := srv.auth.CreateUser("Lock Transition")
	if err != nil {
		t.Fatal(err)
	}
	control := seedTestControl(t, srv, user.ID, ts.URL)
	session, _, err := srv.auth.IssueSession(user.ID, "1h")
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: sessionCookieName, Value: session}
	deviceID := startMCPTestNode(t, srv, ts)
	waitTestNodeLock(t, srv, deviceID)
	challenge := controlChallengeForTest(t, ts, cookie, deviceID)
	clientPrivate := randomTestKey(t)
	clientPublic, err := crypto.X25519Public(clientPrivate)
	if err != nil {
		t.Fatal(err)
	}
	openBody, _ := json.Marshal(map[string]string{
		"deviceId": deviceID, "challenge": challenge, "clientId": control.ID,
		"publicKey": clientPublic,
		"signature": control.sign(crypto.SessionPayload(challenge, deviceID, control.ID, clientPublic)),
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/control/open", strings.NewReader(string(openBody)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	res, raw := oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("control open = %d: %s", res.StatusCode, raw)
	}
	var ready controlReadyTest
	_ = json.Unmarshal(raw, &ready)
	conn := dialControlPipe(t, ts, cookie, ready.SessionID)

	keyRecord, _ := testAPIKeyMaterial(t, "lock_transition_key", []string{"read"}, time.Time{})
	keyRecord.UserID = user.ID
	if err := srv.auth.PutAPIKey(keyRecord); err != nil {
		t.Fatal(err)
	}
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/v1/workspaces/ws_default/authority", nil)
	req.AddCookie(cookie)
	res, raw = oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("authority status = %d: %s", res.StatusCode, raw)
	}
	var status struct {
		Hash    string `json:"hash"`
		Parents []struct {
			Hash       string `json:"hash"`
			Generation uint64 `json:"generation"`
		} `json:"parents"`
	}
	_ = json.Unmarshal(raw, &status)
	if len(status.Parents) == 0 {
		t.Fatalf("no authority parent: %s", raw)
	}
	transitions := make([]map[string]any, 0, len(status.Parents))
	for _, parent := range status.Parents {
		transitions = append(transitions, map[string]any{
			"fromHash": parent.Hash, "generation": parent.Generation,
			"signature": control.sign("rc-authority-v3\n" + itoa(int(parent.Generation)) + "\n" + parent.Hash + "\n" + status.Hash),
		})
	}
	syncBody, _ := json.Marshal(map[string]any{"clientId": control.ID, "transitions": transitions})
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/workspaces/ws_default/authority/sync", strings.NewReader(string(syncBody)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	res, raw = oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("authority sync = %d: %s", res.StatusCode, raw)
	}

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var payload []byte
	err = websocket.Message.Receive(conn, &payload)
	if err == nil {
		t.Fatal("lock transition left the live control pipe open")
	}
	if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("lock transition did not close the live control pipe")
	}
	if _, ok := srv.control.get(ready.SessionID); ok {
		t.Fatal("lock transition left the control session registered")
	}
}
