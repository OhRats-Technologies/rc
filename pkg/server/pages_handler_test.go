package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicPagesAndSetupStatus(t *testing.T) {
	srv := New(Config{Addr: ":0", DataDir: t.TempDir(), PublicURL: "https://rc.example.test", SetupToken: "setup-secret"})
	ts := httptest.NewServer(srv.httpSrv.Handler)
	defer ts.Close()
	defer srv.Close()

	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, res)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "RC / REMOTE CONTROL") || !strings.Contains(body, "Create RC") {
		t.Fatalf("setup page: %d %s", res.StatusCode, body)
	}

	res, err = http.Get(ts.URL + "/docs")
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, res)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "<title>") {
		t.Fatalf("docs page: %d %s", res.StatusCode, body)
	}

	res, err = http.Get(ts.URL + "/api/v1/status")
	if err != nil {
		t.Fatal(err)
	}
	var status struct {
		SetupRequired   bool `json:"setupRequired"`
		SetupAuthorized bool `json:"setupAuthorized"`
		PublicSignup    bool `json:"publicSignup"`
	}
	if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if !status.SetupRequired || status.SetupAuthorized || status.PublicSignup {
		t.Fatalf("status: %#v", status)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/status", nil)
	req.Header.Set("X-RC-Setup-Token", "setup-secret")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if !status.SetupAuthorized {
		t.Fatalf("authorized status: %#v", status)
	}
}

func TestAuthenticatedPagesAndDeviceRename(t *testing.T) {
	srv := New(Config{Addr: ":0", DataDir: t.TempDir(), PublicURL: "https://rc.example.test"})
	ts := httptest.NewServer(srv.httpSrv.Handler)
	defer ts.Close()
	defer srv.Close()
	user, err := srv.auth.CreateUser("Alice")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := srv.auth.IssueSession(user.ID, "1h")
	if err != nil {
		t.Fatal(err)
	}
	device := &RegisteredDevice{
		ID: "dev_test", WorkspaceID: "ws_default", Name: "Build box", Platform: "linux",
		Arch: "amd64", Hostname: "build", Version: "1.0.0", Online: true,
		IdentityPublicKey: base64URL(32, 5), TransportPublicKey: base64URL(32, 6),
	}
	if err := srv.registry.persistEnrollment(device); err != nil {
		t.Fatal(err)
	}
	srv.registry.mu.Lock()
	srv.registry.devices[device.ID] = device
	srv.registry.mu.Unlock()
	cookie := &http.Cookie{Name: sessionCookieName, Value: token}

	for _, target := range []string{"/devices", "/devices/enroll", "/devices/dev_test", "/account"} {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+target, nil)
		req.AddCookie(cookie)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body := readBody(t, res)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s: %d %s", target, res.StatusCode, body)
		}
	}

	req, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/devices/dev_test", strings.NewReader(`{"name":"New name"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, res)
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "New name") {
		t.Fatalf("rename: %d %s", res.StatusCode, body)
	}
}

func TestPageAssetsUseRevalidationUnlessContentHashed(t *testing.T) {
	if contentHashed("styles.css") || contentHashed("app.js") {
		t.Fatal("stable names must not be treated as content hashed")
	}
	for _, name := range []string{"chunk-3csbw2v2.js", "app.abc123.js", "styles.0123456789ab.css"} {
		if !contentHashed(name) {
			t.Fatalf("expected content hash: %s", name)
		}
	}
}

func TestSignupOptionsRequiresTurnstile(t *testing.T) {
	verifier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("secret") != "turnstile-secret" || r.Form.Get("response") != "valid-token" {
			_, _ = w.Write([]byte(`{"success":false,"error-codes":["invalid-input-response"]}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer verifier.Close()
	previous := turnstileEndpoint
	turnstileEndpoint = verifier.URL
	t.Cleanup(func() { turnstileEndpoint = previous })

	srv := New(Config{
		Addr:               ":0",
		DataDir:            t.TempDir(),
		PublicURL:          "https://rc.example.test",
		PublicSignup:       true,
		TurnstileSiteKey:   "turnstile-site",
		TurnstileSecretKey: "turnstile-secret",
	})
	ts := httptest.NewServer(srv.httpSrv.Handler)
	defer ts.Close()
	defer srv.Close()
	if _, err := srv.auth.CreateUser("Alice"); err != nil {
		t.Fatal(err)
	}

	res, payload := postJSONAuth(t, ts.URL+"/api/v1/auth/signup/options", []byte(`{"name":"Bob","turnstile":"invalid"}`), nil, [2]string{})
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("invalid turnstile: %d %s", res.StatusCode, payload)
	}
	res, payload = postJSONAuth(t, ts.URL+"/api/v1/auth/signup/options", []byte(`{"name":"Bob","turnstile":"valid-token"}`), nil, [2]string{})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("valid turnstile: %d %s", res.StatusCode, payload)
	}
	var result struct {
		CeremonyID string          `json:"ceremonyId"`
		Options    json.RawMessage `json:"options"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	if result.CeremonyID == "" || len(result.Options) == 0 {
		t.Fatalf("signup result: %s", payload)
	}
}

func TestVerifyTurnstileRejectsEmptyToken(t *testing.T) {
	srv := &Server{cfg: Config{TurnstileSecretKey: "secret"}}
	if err := srv.verifyTurnstile(context.Background(), "  "); err == nil || !strings.Contains(err.Error(), "complete the human verification") {
		t.Fatalf("empty token: %v", err)
	}
}

func readBody(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	var builder strings.Builder
	buf := make([]byte, 4096)
	for {
		count, err := res.Body.Read(buf)
		builder.Write(buf[:count])
		if err != nil {
			break
		}
	}
	return builder.String()
}
