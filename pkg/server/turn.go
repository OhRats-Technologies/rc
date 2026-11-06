package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

// turnEndpoint is a variable so tests can substitute a local credential API.
var turnEndpoint = "https://rtc.live.cloudflare.com/v1/turn/keys/%s/credentials/generate-ice-servers"

type turnProvider struct {
	tokenID  string
	apiToken string
	client   *http.Client

	mu      sync.Mutex
	cached  []protocol.IceServer
	expires time.Time
}

func newTurnProvider(tokenID, apiToken string) *turnProvider {
	return &turnProvider{
		tokenID:  strings.TrimSpace(tokenID),
		apiToken: strings.TrimSpace(apiToken),
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

// iceServers returns cached short-lived TURN credentials or the public STUN
// fallback when no Cloudflare credentials are configured.
func (t *turnProvider) iceServers(ctx context.Context) ([]protocol.IceServer, error) {
	fallback := []protocol.IceServer{{URLs: []string{"stun:stun.cloudflare.com:3478"}}}
	if t.tokenID == "" || t.apiToken == "" {
		return fallback, nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.cached) > 0 && time.Now().Before(t.expires) {
		return append([]protocol.IceServer(nil), t.cached...), nil
	}
	body, err := json.Marshal(map[string]int{"ttl": 3600})
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf(turnEndpoint, t.tokenID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.apiToken)
	res, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Cloudflare TURN credential request failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("Cloudflare TURN credential request failed: %s", res.Status)
	}
	var parsed struct {
		IceServers []protocol.IceServer `json:"iceServers"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("Cloudflare TURN credential response: %w", err)
	}
	if len(parsed.IceServers) == 0 {
		return fallback, nil
	}
	t.cached = append([]protocol.IceServer(nil), parsed.IceServers...)
	t.expires = time.Now().Add(50 * time.Minute)
	return append([]protocol.IceServer(nil), t.cached...), nil
}
