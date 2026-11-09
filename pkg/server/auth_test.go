package server

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPasskeySetupLoginMeAndSSE(t *testing.T) {
	srv, ts := newAuthServer(t)
	fixture, rp := loadFixture(t)
	userID := "rc-fixture-user"

	// Setup ceremony with the deterministic fixture challenge.
	state, err := stateFor([]byte("xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"), rp, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	_ = state
	challenge, _ := base64.RawURLEncoding.DecodeString(fixture.Registration.Challenge)
	setupState, err := stateFor(challenge, rp, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.auth.PutCeremony("setup-1", "setup", userID, setupState, futureMs()); err != nil {
		t.Fatal(err)
	}
	registrationRaw, credential := registrationJSON(t, fixture)
	body, _ := json.Marshal(map[string]any{"ceremonyId": "setup-1", "response": json.RawMessage(registrationRaw)})
	res, setupPayload := postJSONAuth(t, ts.URL+"/api/v1/auth/setup/verify", body, nil, [2]string{})
	if res.StatusCode != http.StatusCreated {
		payload := new(bytes.Buffer)
		_, _ = payload.ReadFrom(res.Body)
		t.Fatalf("setup verify status %d: %s", res.StatusCode, setupPayload)
	}
	cookie := sessionCookie(t, res)
	if credential.ID == nil {
		t.Fatal("fixture credential missing")
	}

	res, payload := postJSONAuth(t, ts.URL+"/api/v1/auth/setup/options", []byte(`{"name":"Alice"}`), nil, [2]string{})
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("setup after completion = %d: %s", res.StatusCode, payload)
	}

	// /me, devices, and SSE require the session cookie.
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/me", nil)
	req.AddCookie(cookie)
	res, err = http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("me status %v: %v", res.StatusCode, err)
	}
	var me struct {
		User struct{ ID, Name string } `json:"user"`
	}
	_ = json.NewDecoder(res.Body).Decode(&me)
	res.Body.Close()
	if me.User.Name != "Alice" || me.User.ID != userID {
		t.Fatalf("unexpected me: %+v", me)
	}

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/v1/devices", nil)
	req.AddCookie(cookie)
	res, err = http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("devices with session = %v, %v", res.StatusCode, err)
	}
	res.Body.Close()

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/v1/events", nil)
	req.AddCookie(cookie)
	res, err = http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("events with session = %v, %v", res.StatusCode, err)
	}
	scanner := bufio.NewScanner(res.Body)
	if !scanner.Scan() || !strings.HasPrefix(scanner.Text(), ": ping") {
		t.Fatalf("SSE did not send a ping: %q", scanner.Text())
	}
	res.Body.Close()

	// Login ceremony replays the fixture assertion.
	authChallenge, _ := base64.RawURLEncoding.DecodeString(fixture.Authentication.Challenge)
	loginState, _ := stateFor(authChallenge, rp, "")
	if err := srv.auth.PutCeremony("login-1", "login", "", loginState, futureMs()); err != nil {
		t.Fatal(err)
	}
	loginBody, _ := json.Marshal(map[string]any{"ceremonyId": "login-1", "response": json.RawMessage(authenticationJSON(t, fixture)), "lifetime": "1h"})
	res, payload = postJSONAuth(t, ts.URL+"/api/v1/auth/login/verify", loginBody, nil, [2]string{})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login verify = %d: %s", res.StatusCode, payload)
	}
	if sessionCookie(t, res) == nil {
		t.Fatal("login did not set a session")
	}
}

func TestPublicSignupVerifyCreatesAccount(t *testing.T) {
	srv := New(Config{
		Addr: ":0", DataDir: t.TempDir(), PublicURL: "https://rc.example.test",
		PublicSignup: true, TurnstileSiteKey: "site", TurnstileSecretKey: "secret",
	})
	ts := httptest.NewServer(srv.httpSrv.Handler)
	defer ts.Close()
	defer srv.Close()
	if _, err := srv.auth.CreateUser("Alice"); err != nil {
		t.Fatal(err)
	}
	fixture, rp := loadFixture(t)
	challenge, _ := base64.RawURLEncoding.DecodeString(fixture.Registration.Challenge)
	state, err := stateFor(challenge, rp, "Bob")
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.auth.PutCeremony("signup-1", "signup", "signup-user", state, futureMs()); err != nil {
		t.Fatal(err)
	}
	registration, _ := registrationJSON(t, fixture)
	body, _ := json.Marshal(map[string]any{"ceremonyId": "signup-1", "response": json.RawMessage(registration)})
	res, payload := postJSONAuth(t, ts.URL+"/api/v1/auth/register/verify", body, nil, [2]string{})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("signup verify = %d: %s", res.StatusCode, payload)
	}
	if sessionCookie(t, res) == nil {
		t.Fatal("signup did not set a session")
	}
	user, err := srv.auth.GetUser("signup-user")
	if err != nil || user.Name != "Bob" {
		t.Fatalf("created user: %#v %v", user, err)
	}
}

func TestSetupTokenAndEnrollment(t *testing.T) {
	srv := New(Config{Addr: ":0", DataDir: t.TempDir(), PublicURL: "https://rc.example.test", SetupToken: "setup-secret"})
	ts := httptest.NewServer(srv.httpSrv.Handler)
	defer ts.Close()
	defer srv.Close()

	res, _ := postJSONAuth(t, ts.URL+"/api/v1/auth/setup/options", []byte(`{"name":"Alice"}`), nil, [2]string{})
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("setup without token = %d", res.StatusCode)
	}
	res, _ = postJSONAuth(t, ts.URL+"/api/v1/auth/setup/options", []byte(`{"name":"Alice"}`), nil, [2]string{"X-RC-Setup-Token", "setup-secret"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("setup with token = %d", res.StatusCode)
	}

	// Seed a user and session directly, then create and consume an enrollment token.
	user, err := srv.auth.CreateUser("Alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.auth.EnsureWorkspaceMembership(defaultWorkspaceID, user.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	token, _, err := srv.auth.IssueSession(user.ID, "1h")
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: sessionCookieName, Value: token}
	res, payload := postJSONAuth(t, ts.URL+"/api/v1/workspaces/ws_default/enrollments", []byte(`{}`), cookie, [2]string{})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("enrollment create = %d: %s", res.StatusCode, payload)
	}
	var created struct {
		Token          string `json:"token"`
		Install        string `json:"install"`
		Enroll         string `json:"enroll"`
		InstallWindows string `json:"installWindows"`
		EnrollWindows  string `json:"enrollWindows"`
	}
	_ = json.Unmarshal(payload, &created)
	if created.Token == "" {
		t.Fatal("empty enrollment token")
	}
	if !strings.Contains(created.Install, "install.sh") || !strings.Contains(created.Enroll, "rc enroll") ||
		!strings.Contains(created.InstallWindows, "install.ps1") || !strings.Contains(created.EnrollWindows, "rc.exe") {
		t.Fatalf("enrollment commands: %s", payload)
	}
	enrollPayload := validEnrollPayload(created.Token, "passkey-node")
	res, payload = postJSONAuth(t, ts.URL+"/api/v1/node/enroll", mustJSON(t, enrollPayload), nil, [2]string{})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("enroll with durable token = %d: %s", res.StatusCode, payload)
	}
	res, payload = postJSONAuth(t, ts.URL+"/api/v1/node/enroll", mustJSON(t, validEnrollPayload(created.Token, "replay")), nil, [2]string{})
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("replayed token = %d: %s", res.StatusCode, payload)
	}
}

func TestLogoutClearsSession(t *testing.T) {
	srv, ts := newAuthServer(t)
	user, _ := srv.auth.CreateUser("Alice")
	token, _, _ := srv.auth.IssueSession(user.ID, "1h")
	cookie := &http.Cookie{Name: sessionCookieName, Value: token}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/account/logout", strings.NewReader(""))
	req.AddCookie(cookie)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil || res.StatusCode != http.StatusSeeOther {
		t.Fatalf("logout = %v, %v", res.StatusCode, err)
	}
	res.Body.Close()
	if _, err := srv.auth.LookupSession(token); err == nil {
		t.Fatal("session was not revoked")
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func futureMs() int64 {
	return time.Now().Add(time.Hour).UnixMilli()
}
