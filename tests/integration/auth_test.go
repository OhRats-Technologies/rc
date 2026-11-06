package integration

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

func TestServerPasskeySessionRoutes(t *testing.T) {
	dataDir := t.TempDir()
	store, err := authstore.Open(filepath.Join(dataDir, "rc.db"))
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser("Alice")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := store.IssueSession(user.ID, "1h")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	srv := startServer(t,
		"RC_DATA_DIR="+dataDir,
		"PUBLIC_URL=https://rc.example.test",
		"RC_SETUP_TOKEN=setup-secret",
	)
	cookie := &http.Cookie{Name: "rc_session", Value: token}

	req, _ := http.NewRequest(http.MethodGet, srv.baseURL+"/api/v1/me", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me without session = %v, %v", res.StatusCode, err)
	}
	res.Body.Close()

	req, _ = http.NewRequest(http.MethodGet, srv.baseURL+"/api/v1/me", nil)
	req.AddCookie(cookie)
	res, err = http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("me with session = %v, %v", res.StatusCode, err)
	}
	var me struct {
		User struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"user"`
	}
	_ = json.NewDecoder(res.Body).Decode(&me)
	res.Body.Close()
	if me.User.Name != "Alice" || me.User.ID != user.ID {
		t.Fatalf("unexpected me: %s", jsonString(t, me))
	}

	req, _ = http.NewRequest(http.MethodGet, srv.baseURL+"/api/v1/events", nil)
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

	// Protected first-instance setup requires the configured token.
	body := []byte(`{"name":"Alice"}`)
	req, _ = http.NewRequest(http.MethodPost, srv.baseURL+"/api/v1/auth/setup/options", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusForbidden {
		t.Fatalf("setup without token = %v, %v", res.StatusCode, err)
	}
	res.Body.Close()
	req, _ = http.NewRequest(http.MethodPost, srv.baseURL+"/api/v1/auth/setup/options", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-RC-Setup-Token", "setup-secret")
	res, err = http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != http.StatusConflict {
		// Setup is already complete because the test seeded a user.
		t.Fatalf("setup with token = %v, %v", res.StatusCode, err)
	}
	res.Body.Close()
}
