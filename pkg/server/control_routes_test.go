package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/crypto"
	"github.com/OhRats-Technologies/rc/pkg/protocol"
	"github.com/OhRats-Technologies/rc/pkg/transport"
)

func TestControlHelperProcess(t *testing.T) {
	if os.Getenv("RC_CONTROL_HELPER") != "1" {
		return
	}
	_, _ = os.Stdout.WriteString("control-ok")
}

type controlReadyTest struct {
	SessionID          string                       `json:"sessionId"`
	TransportPublicKey string                       `json:"transportPublicKey"`
	EphemeralPublicKey string                       `json:"ephemeralPublicKey"`
	Signature          string                       `json:"signature"`
	IceServers         []protocol.IceServer         `json:"iceServers"`
	Attempts           []protocol.ControlIceAttempt `json:"attempts"`
}

func TestAuthenticatedControlSignalingAndEncryptedProcess(t *testing.T) {
	srv, ts := newAuthServer(t)
	user, err := srv.auth.CreateUser("Control Browser")
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
	payload := crypto.SessionPayload(challenge, deviceID, control.ID, clientPublic)
	body, _ := json.Marshal(map[string]string{
		"deviceId": deviceID, "challenge": challenge, "clientId": control.ID,
		"publicKey": clientPublic, "signature": control.sign(payload),
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/control/open", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	res, raw := oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("control open = %d: %s", res.StatusCode, raw)
	}
	var ready controlReadyTest
	if err := json.Unmarshal(raw, &ready); err != nil || ready.SessionID == "" || ready.Signature == "" {
		t.Fatalf("control ready = %+v, %v", ready, err)
	}
	key, err := crypto.DeriveClientKey(clientPrivate, ready.TransportPublicKey,
		ready.EphemeralPublicKey, challenge, deviceID, control.ID)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := transport.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	offer, err := peer.Offer()
	if err != nil {
		t.Fatal(err)
	}
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
	if err := json.Unmarshal(raw, &answer); err != nil || answer.SDP == "" {
		t.Fatalf("webrtc answer = %s", raw)
	}
	if err := peer.AcceptAnswer(answer.SDP); err != nil {
		t.Fatal(err)
	}
	if err := peer.WaitOpen(10 * time.Second); err != nil {
		t.Fatal(err)
	}

	helperEnabled := "1"
	start, _ := json.Marshal(protocol.ControlMessage{
		Type: protocol.TypeProcessStart, ID: "control_process",
		Mode: &protocol.ExecutionMode{
			Kind: protocol.ExecutionModeArgv, Program: os.Args[0],
			Args: []string{"-test.run=TestControlHelperProcess"},
		},
		Environment: &protocol.EnvironmentSpec{
			Base:    protocol.EnvironmentBaseInherit,
			Changes: []protocol.EnvironmentChange{{Name: "RC_CONTROL_HELPER", Value: &helperEnabled}},
		},
	})
	ciphertext, err := crypto.EncryptFrame(&key, 1, 1, ready.SessionID, "c2n", start)
	if err != nil {
		t.Fatal(err)
	}
	frame, _ := json.Marshal(protocol.ControlTransportMessage{
		Type: protocol.TypeControlFrame, SessionID: ready.SessionID,
		Sequence: 1, Ciphertext: ciphertext,
	})
	if err := peer.Send(frame); err != nil {
		t.Fatal(err)
	}
	foundOutput := false
	receivedExit := false
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !receivedExit {
		message, err := peer.Receive(time.Second)
		if err != nil {
			continue
		}
		var frame protocol.ControlTransportMessage
		if json.Unmarshal(message, &frame) != nil || frame.Type != protocol.TypeControlFrame {
			t.Fatalf("unexpected control frame = %s", message)
		}
		plain, err := crypto.DecryptFrame(&key, 2, frame.Sequence, ready.SessionID, "n2c", frame.Ciphertext)
		if err != nil {
			t.Fatal(err)
		}
		var controlMessage protocol.ControlMessage
		if json.Unmarshal(plain, &controlMessage) != nil {
			t.Fatal("invalid control message")
		}
		if controlMessage.Type == protocol.TypeProcessStdout {
			data, _ := base64.RawURLEncoding.DecodeString(controlMessage.Data)
			foundOutput = foundOutput || strings.Contains(string(data), "control-ok")
		}
		receivedExit = receivedExit || controlMessage.Type == protocol.TypeProcessExit
	}
	if !foundOutput || !receivedExit {
		t.Fatalf("control output=%v exit=%v", foundOutput, receivedExit)
	}
}

func controlChallengeForTest(t *testing.T, ts *httptest.Server, cookie *http.Cookie, deviceID string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"deviceId": deviceID})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/control/challenge", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	res, raw := oauthDo(t, http.DefaultClient, req)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("control challenge = %d: %s", res.StatusCode, raw)
	}
	var response struct {
		Challenge string `json:"challenge"`
	}
	if err := json.Unmarshal(raw, &response); err != nil || response.Challenge == "" {
		t.Fatalf("challenge = %s", raw)
	}
	return response.Challenge
}

func randomTestKey(t *testing.T) string {
	t.Helper()
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(value)
}

func TestAPIKeyControlOpenUsesRCLockAuthority(t *testing.T) {
	srv, ts := newAuthServer(t)
	user, err := srv.auth.CreateUser("Control API")
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.auth.EnsureWorkspaceMembership(defaultWorkspaceID, user.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	key, private := testAPIKeyMaterial(t, "control_api_key", []string{"execute"}, time.Time{})
	key.UserID = user.ID
	if err := srv.auth.PutAPIKey(key); err != nil {
		t.Fatal(err)
	}
	deviceID := startMCPTestNode(t, srv, ts)
	waitTestNodeLock(t, srv, deviceID)

	challengeBody, _ := json.Marshal(map[string]string{"deviceId": deviceID})
	challengeRequest := signedAPIRequest(t, http.MethodPost,
		ts.URL+"/api/v1/control/challenge", challengeBody, key.ID, private, "control-challenge-01")
	challengeRequest.Header.Set("Content-Type", "application/json")
	res, raw := oauthDo(t, http.DefaultClient, challengeRequest)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("API key challenge = %d: %s", res.StatusCode, raw)
	}
	var challengeResponse struct {
		Challenge string `json:"challenge"`
	}
	if err := json.Unmarshal(raw, &challengeResponse); err != nil {
		t.Fatal(err)
	}
	clientPrivate := randomTestKey(t)
	clientPublic, err := crypto.X25519Public(clientPrivate)
	if err != nil {
		t.Fatal(err)
	}
	sessionPayload := crypto.SessionPayload(challengeResponse.Challenge, deviceID, key.ID, clientPublic)
	signature := base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, []byte(sessionPayload)))
	openBody, _ := json.Marshal(map[string]string{
		"deviceId": deviceID, "challenge": challengeResponse.Challenge,
		"clientId": key.ID, "publicKey": clientPublic, "signature": signature,
	})
	openRequest := signedAPIRequest(t, http.MethodPost,
		ts.URL+"/api/v1/control/open", openBody, key.ID, private, "control-open-000001")
	openRequest.Header.Set("Content-Type", "application/json")
	res, raw = oauthDo(t, http.DefaultClient, openRequest)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("API key control open = %d: %s", res.StatusCode, raw)
	}
	var ready controlReadyTest
	if err := json.Unmarshal(raw, &ready); err != nil || ready.SessionID == "" {
		t.Fatalf("API key control ready = %s", raw)
	}
}
