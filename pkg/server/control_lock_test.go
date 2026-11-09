package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/crypto"
	"github.com/OhRats-Technologies/rc/pkg/transport"
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
	peer, err := transport.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	offer, _ := peer.Offer()
	webrtcBody, _ := json.Marshal(map[string]any{"deviceId": deviceID, "sdp": offer, "mode": "host"})
	req, _ = http.NewRequest(http.MethodPost,
		ts.URL+"/api/v1/control/"+ready.SessionID+"/webrtc", strings.NewReader(string(webrtcBody)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	res, raw = oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("control webrtc = %d: %s", res.StatusCode, raw)
	}
	var answer struct {
		SDP string `json:"sdp"`
	}
	_ = json.Unmarshal(raw, &answer)
	if err := peer.AcceptAnswer(answer.SDP); err != nil {
		t.Fatal(err)
	}
	if err := peer.WaitOpen(10 * time.Second); err != nil {
		t.Fatal(err)
	}

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

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		checkBody, _ := json.Marshal(map[string]any{"deviceId": deviceID, "sdp": "v=0", "mode": "host"})
		req, _ = http.NewRequest(http.MethodPost,
			ts.URL+"/api/v1/control/"+ready.SessionID+"/webrtc", strings.NewReader(string(checkBody)))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		res, _ = oauthDo(t, http.DefaultClient, req)
		if res.StatusCode == http.StatusNotFound {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("lock transition did not invalidate the live control transport")
}
