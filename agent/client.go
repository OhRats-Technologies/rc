package main

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var (
	errNodeRemoved         = errors.New("node removed from RC")
	errLockedServerMissing = errors.New("locked RC Node is no longer recognized by the server")
)

func enroll(serverURL, token, displayName string) (state, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return state{}, err
	}
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	transportPrivate, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return state{}, err
	}
	if err != nil {
		return state{}, err
	}
	hostname, _ := os.Hostname()
	if displayName == "" {
		displayName = hostname
	}
	payload := enrollRequest{
		Token: token, Name: displayName, Hostname: hostname, Platform: runtime.GOOS, Arch: runtime.GOARCH,
		PublicKey:          string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})),
		TransportPublicKey: base64.RawURLEncoding.EncodeToString(transportPrivate.PublicKey().Bytes()),
		AgentVersion:       version, Capabilities: []string{"process", "update", "lock", "e2e", "webrtc"},
	}
	data, _ := json.Marshal(payload)
	resp, err := http.Post(strings.TrimRight(serverURL, "/")+"/api/v1/agent/enroll", "application/json", bytes.NewReader(data))
	if err != nil {
		return state{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return state{}, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var out enrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return state{}, err
	}
	return state{DeviceID: out.DeviceID, PrivateKey: base64.RawStdEncoding.EncodeToString(priv),
		TransportPrivateKey: base64.RawURLEncoding.EncodeToString(transportPrivate.Bytes()),
		TransportPublicKey:  base64.RawURLEncoding.EncodeToString(transportPrivate.PublicKey().Bytes())}, nil
}

func signedAgentRequest(serverURL, method, path string, value state) (*http.Request, error) {
	privateBytes, err := base64.RawStdEncoding.DecodeString(value.PrivateKey)
	if err != nil || len(privateBytes) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid stored private key")
	}
	u, err := url.Parse(strings.TrimRight(serverURL, "/"))
	if err != nil {
		return nil, err
	}
	challengeBody, _ := json.Marshal(map[string]string{"device": value.DeviceID})
	challengeResp, err := http.Post(strings.TrimRight(serverURL, "/")+"/api/v1/agent/challenge", "application/json", bytes.NewReader(challengeBody))
	if err != nil {
		return nil, err
	}
	defer challengeResp.Body.Close()
	if challengeResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(challengeResp.Body, 4096))
		return nil, fmt.Errorf("agent challenge: %s: %s", challengeResp.Status, strings.TrimSpace(string(body)))
	}
	var auth struct {
		Challenge string `json:"challenge"`
	}
	if err := json.NewDecoder(challengeResp.Body).Decode(&auth); err != nil || auth.Challenge == "" {
		return nil, errors.New("invalid agent challenge")
	}
	u.Path = path
	query := u.Query()
	query.Set("device", value.DeviceID)
	u.RawQuery = query.Encode()
	payload := "rc-auth-v2\n" + value.DeviceID + "\n" + auth.Challenge + "\n" + method + "\n" + path
	signature := ed25519.Sign(ed25519.PrivateKey(privateBytes), []byte(payload))
	req, err := http.NewRequest(method, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-RC-Challenge", auth.Challenge)
	req.Header.Set("X-RC-Signature", base64.RawURLEncoding.EncodeToString(signature))
	return req, nil
}

func unregister(serverURL string, value state) error {
	req, err := signedAgentRequest(serverURL, http.MethodDelete, "/api/v1/agent/self", value)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
}

func connect(ctx context.Context, serverURL string, value state, stateDir string, manager *processManager) error {
	return connectWithLiveness(ctx, serverURL, value, stateDir, manager, 10*time.Second, 30*time.Second)
}

func connectWithLiveness(ctx context.Context, serverURL string, value state, stateDir string, manager *processManager, heartbeatInterval, livenessTimeout time.Duration) error {
	connectionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := signedAgentRequest(serverURL, http.MethodGet, "/api/v1/agent/ws", value)
	if err != nil {
		return err
	}
	u := req.URL
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	conn, response, err := websocket.DefaultDialer.Dial(u.String(), req.Header)
	if err != nil {
		if response != nil && response.StatusCode == http.StatusGone {
			_ = os.Remove(lockPath(stateDir))
			_ = os.Remove(statePath(stateDir))
			return errNodeRemoved
		}
		if response != nil && response.StatusCode == http.StatusNotFound {
			if lockHash(stateDir) != "" {
				return errLockedServerMissing
			}
			_ = os.Remove(statePath(stateDir))
			return errNodeRemoved
		}
		return err
	}
	defer conn.Close()
	conn.SetReadLimit(maxWireMessageBytes)
	var writeMu sync.Mutex
	send := func(message wireMessage) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(livenessTimeout))
		return conn.WriteJSON(message)
	}
	sendPing := func() error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(livenessTimeout))
	}
	refreshReadDeadline := func(string) error {
		return conn.SetReadDeadline(time.Now().Add(livenessTimeout))
	}
	conn.SetPongHandler(refreshReadDeadline)
	if err := refreshReadDeadline(""); err != nil {
		return err
	}
	hostname, _ := os.Hostname()
	lock, _ := loadLock(stateDir)
	if err := send(wireMessage{
		Type: "hello", AgentVersion: version, Hostname: hostname,
		Platform: runtime.GOOS, Arch: runtime.GOARCH, Capabilities: []string{"process", "update", "lock", "e2e", "webrtc"},
		TransportPublicKey: value.TransportPublicKey, LockHash: lockHash(stateDir), LockGeneration: lock.Generation,
	}); err != nil {
		return err
	}

	readDone := make(chan error, 1)
	manager.attach(send)
	defer manager.detach()
	control := newControlManager(value, stateDir, serverURL, manager, send)
	go func() {
		for {
			var message wireMessage
			if err := conn.ReadJSON(&message); err != nil {
				readDone <- err
				return
			}
			if strings.HasPrefix(message.Type, "control.") || strings.HasPrefix(message.Type, "lock.") {
				if err := control.handle(message); err != nil {
					if errors.Is(err, errNodeRemoved) {
						readDone <- err
						return
					}
					_ = send(wireMessage{Type: "control.error", RequestID: message.RequestID, Output: err.Error()})
				}
				continue
			}
			if message.Type == "process.permit" {
				_ = control.handle(message)
				continue
			}
			if message.Type == "mcp.process.start" {
				if err := handleMcpProcess(stateDir, value.DeviceID, manager, message); err != nil {
					_ = send(wireMessage{Type: "process.exit", ID: message.ID, Output: err.Error()})
				}
				continue
			}
			if strings.HasPrefix(message.Type, "ssh.process.") {
				if err := handleSshProcess(stateDir, manager, message); err != nil {
					_ = send(wireMessage{Type: "process.exit", ID: message.ID, Output: err.Error()})
				}
				continue
			}
			if strings.HasPrefix(message.Type, "process.") || message.Type == "node.update" {
				continue
			}
			manager.handle(message)
		}
	}()

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-connectionCtx.Done():
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shutdown"), time.Now().Add(time.Second))
			return nil
		case err := <-readDone:
			return err
		case <-ticker.C:
			if err := sendPing(); err != nil {
				return err
			}
			if err := send(wireMessage{Type: "heartbeat"}); err != nil {
				return err
			}
		}
	}
}

type remoteNodeStatus struct {
	Name         string `json:"name"`
	Online       bool   `json:"online"`
	AgentVersion string `json:"agentVersion"`
}

func fetchStatus(serverURL string, value state) (remoteNodeStatus, error) {
	req, err := signedAgentRequest(serverURL, http.MethodGet, "/api/v1/agent/self", value)
	if err != nil {
		return remoteNodeStatus{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return remoteNodeStatus{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
		return remoteNodeStatus{}, errNodeRemoved
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return remoteNodeStatus{}, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var out remoteNodeStatus
	err = json.NewDecoder(resp.Body).Decode(&out)
	return out, err
}
