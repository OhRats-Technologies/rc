package main

import (
	"bytes"
	"context"
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

var errNodeRemoved = errors.New("node removed from RC")

func enroll(serverURL, token, displayName string) (state, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return state{}, err
	}
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return state{}, err
	}
	hostname, _ := os.Hostname()
	if displayName == "" {
		displayName = hostname
	}
	payload := enrollRequest{
		Token: token, Name: displayName, Hostname: hostname, Platform: runtime.GOOS, Arch: runtime.GOARCH,
		PublicKey:    string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})),
		AgentVersion: version, Capabilities: []string{"process", "update"},
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
	return state{DeviceID: out.DeviceID, PrivateKey: base64.RawStdEncoding.EncodeToString(priv)}, nil
}

func signedURL(serverURL, path string, value state) (*url.URL, error) {
	privateBytes, err := base64.RawStdEncoding.DecodeString(value.PrivateKey)
	if err != nil || len(privateBytes) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid stored private key")
	}
	u, err := url.Parse(strings.TrimRight(serverURL, "/"))
	if err != nil {
		return nil, err
	}
	u.Path = path
	ts := fmt.Sprintf("%d", time.Now().Unix())
	signature := ed25519.Sign(ed25519.PrivateKey(privateBytes), []byte("rc:"+value.DeviceID+":"+ts))
	query := u.Query()
	query.Set("device", value.DeviceID)
	query.Set("ts", ts)
	query.Set("sig", base64.RawURLEncoding.EncodeToString(signature))
	u.RawQuery = query.Encode()
	return u, nil
}

func unregister(serverURL string, value state) error {
	u, err := signedURL(serverURL, "/api/v1/agent/self", value)
	if err != nil {
		return err
	}
	req, _ := http.NewRequest(http.MethodDelete, u.String(), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
}

func connect(ctx context.Context, serverURL string, value state, stateDir string, manager *processManager) error {
	connectionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	u, err := signedURL(serverURL, "/api/v1/agent/ws", value)
	if err != nil {
		return err
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	conn, response, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		if response != nil && response.StatusCode == http.StatusNotFound {
			_ = os.Remove(statePath(stateDir))
			return errNodeRemoved
		}
		return err
	}
	defer conn.Close()
	var writeMu sync.Mutex
	send := func(message wireMessage) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(message)
	}
	hostname, _ := os.Hostname()
	if err := send(wireMessage{
		Type: "hello", AgentVersion: version, Hostname: hostname,
		Platform: runtime.GOOS, Arch: runtime.GOARCH, Capabilities: []string{"process", "update"},
	}); err != nil {
		return err
	}

	readDone := make(chan error, 1)
	manager.attach(send)
	defer manager.detach()
	go func() {
		for {
			var message wireMessage
			if err := conn.ReadJSON(&message); err != nil {
				readDone <- err
				return
			}
			if message.Type == "node.update" {
				fmt.Printf("Updating OhRats RC Node %s…\n", version)
				if err := replaceExecutable(serverURL); err != nil {
					_ = send(wireMessage{Type: "node.update.error", Output: err.Error()})
					fmt.Fprintf(os.Stderr, "update failed: %v\n", err)
					continue
				}
				manager.shutdown()
				_ = send(wireMessage{Type: "node.update.ready", AgentVersion: version})
				fmt.Println("Update installed; restarting RC Node…")
				if err := syscallExecCurrent(); err != nil {
					_ = send(wireMessage{Type: "node.update.error", Output: err.Error()})
					readDone <- fmt.Errorf("restart after update: %w", err)
				}
				return
			}
			if message.Type == "node.remove" {
				manager.shutdown()
				_ = os.Remove(statePath(stateDir))
				readDone <- errNodeRemoved
				return
			}
			manager.handle(message)
		}
	}()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-connectionCtx.Done():
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shutdown"), time.Now().Add(time.Second))
			return nil
		case err := <-readDone:
			return err
		case <-ticker.C:
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
	u, err := signedURL(serverURL, "/api/v1/agent/self", value)
	if err != nil {
		return remoteNodeStatus{}, err
	}
	resp, err := http.Get(u.String())
	if err != nil {
		return remoteNodeStatus{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
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
