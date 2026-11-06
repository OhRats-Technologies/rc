package node

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Heartbeat sends one signed presence update. It is the only periodic HTTP
// call the Node makes; process control never uses this path.
func Heartbeat(ctx context.Context, server string, state *NodeState) error {
	const path = "/api/v1/node/heartbeat"
	auth, err := SignNodeRequest(state, http.MethodPost, path, nil)
	if err != nil {
		return err
	}
	target := strings.TrimRight(server, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, nil)
	if err != nil {
		return err
	}
	for key, value := range auth.Headers() {
		req.Header.Set(key, value)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("server error (%d): %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// ErrNotEnrolled is returned when a local Node identity is required.
var ErrNotEnrolled = errors.New("RC Node is not enrolled; run rc enroll first")
