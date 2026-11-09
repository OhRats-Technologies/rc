package node

import (
	"context"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/apiclient"
)

var NodeCapabilities = []string{"shell", "files", "pty", "webrtc", "execution-v2", "mcp-account-v2"}

type RemoteNodeStatus struct {
	Name    string `json:"name"`
	Online  bool   `json:"online"`
	Version string `json:"version"`
}

type EnrollRequest struct {
	Token              string   `json:"token"`
	Name               string   `json:"name"`
	Hostname           string   `json:"hostname"`
	Platform           string   `json:"platform"`
	Arch               string   `json:"arch"`
	IdentityPublicKey  string   `json:"identityPublicKey"`
	TransportPublicKey string   `json:"transportPublicKey"`
	Version            string   `json:"version"`
	Capabilities       []string `json:"capabilities"`
}

type EnrollResponse struct {
	DeviceID string `json:"deviceId"`
}

func Enroll(ctx context.Context, server, token, displayName, version string) (*NodeState, error) {
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, err
	}
	identityPub := b64URL.EncodeToString(priv.Public().(ed25519.PublicKey))

	transportBytes := make([]byte, 32)
	if _, err := rand.Read(transportBytes); err != nil {
		return nil, err
	}
	k, err := ecdh.X25519().NewPrivateKey(transportBytes)
	if err != nil {
		return nil, err
	}
	transportPub := b64URL.EncodeToString(k.PublicKey().Bytes())

	h := getHostname()
	name := strings.TrimSpace(displayName)
	if name == "" {
		name = h
	}

	req := EnrollRequest{
		Token:              token,
		Name:               name,
		Hostname:           h,
		Platform:           getPlatform(),
		Arch:               getArch(),
		IdentityPublicKey:  identityPub,
		TransportPublicKey: transportPub,
		Version:            version,
		Capabilities:       NodeCapabilities,
	}

	var resp EnrollResponse
	if err := apiclient.PublicPost(ctx, server, "/api/v1/node/enroll", req, &resp); err != nil {
		return nil, err
	}

	return &NodeState{
		V:               StateVersion,
		DeviceID:        resp.DeviceID,
		IdentitySeed:    b64URL.EncodeToString(priv.Seed()),
		TransportSecret: b64URL.EncodeToString(transportBytes),
	}, nil
}

func FetchStatus(ctx context.Context, server string, state *NodeState) (*RemoteNodeStatus, error) {
	path := "/api/v1/node/self"
	auth, err := SignNodeRequest(state, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	target := fmt.Sprintf("%s%s", strings.TrimRight(server, "/"), path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range auth.Headers() {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusGone {
		return nil, errors.New("node removed from RC")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("server error (%d): %s", res.StatusCode, string(body))
	}

	var st RemoteNodeStatus
	if err := json.NewDecoder(res.Body).Decode(&st); err != nil {
		return nil, err
	}
	return &st, nil
}

func getHostname() string {
	h, err := os.Hostname()
	if err != nil || strings.TrimSpace(h) == "" {
		return "localhost"
	}
	return h
}

func getPlatform() string {
	if runtime.GOOS == "darwin" {
		return "darwin"
	}
	return runtime.GOOS
}

func getArch() string {
	if runtime.GOARCH == "amd64" {
		return "amd64"
	}
	if runtime.GOARCH == "arm64" {
		return "arm64"
	}
	return runtime.GOARCH
}
