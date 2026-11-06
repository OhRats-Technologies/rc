package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTurnProviderFallsBackToStunWithoutCredentials(t *testing.T) {
	provider := newTurnProvider("", "")
	servers, err := provider.iceServers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 || servers[0].URLs[0] != "stun:stun.cloudflare.com:3478" {
		t.Fatalf("servers: %#v", servers)
	}
}

func TestTurnProviderCachesCloudflareCredentials(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer api-token" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.Header.Get("Authorization"))
		}
		if !strings.HasSuffix(r.URL.Path, "/credentials/generate-ice-servers") {
			t.Fatalf("path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"iceServers": []map[string]any{{
				"urls":       []string{"turn:turn.cloudflare.com:3478"},
				"username":   "user",
				"credential": "pass",
			}},
		})
	}))
	defer upstream.Close()
	previous := turnEndpoint
	turnEndpoint = upstream.URL + "/v1/turn/keys/%s/credentials/generate-ice-servers"
	t.Cleanup(func() { turnEndpoint = previous })

	provider := newTurnProvider("token-id", "api-token")
	for attempt := 0; attempt < 2; attempt++ {
		servers, err := provider.iceServers(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(servers) != 1 || servers[0].Username != "user" {
			t.Fatalf("servers: %#v", servers)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one upstream call, got %d", calls.Load())
	}
}

func TestTurnProviderReportsUpstreamFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusForbidden)
	}))
	defer upstream.Close()
	previous := turnEndpoint
	turnEndpoint = upstream.URL + "/%s"
	t.Cleanup(func() { turnEndpoint = previous })

	provider := newTurnProvider("token-id", "api-token")
	if _, err := provider.iceServers(context.Background()); err == nil {
		t.Fatal("expected upstream failure")
	}
}
