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
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

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
		AgentVersion: version, Capabilities: []string{"shell"},
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
	signature := ed25519.Sign(ed25519.PrivateKey(privateBytes), []byte("relay:"+value.DeviceID+":"+ts))
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

func connect(ctx context.Context, serverURL string, value state) error {
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
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	var writeMu sync.Mutex
	send := func(message wireMessage) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(message)
	}

	readDone := make(chan error, 1)
	jobs := make(chan wireMessage, 32)
	go func() {
		for {
			var message wireMessage
			if err := conn.ReadJSON(&message); err != nil {
				readDone <- err
				return
			}
			if message.Type != "job" || message.ID == "" {
				continue
			}
			select {
			case jobs <- message:
			case <-connectionCtx.Done():
				return
			}
		}
	}()
	go func() {
		for {
			select {
			case message := <-jobs:
				runJob(connectionCtx, send, message)
			case <-connectionCtx.Done():
				return
			}
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

type chunkWriter struct {
	send  func(wireMessage) error
	jobID string
}

func (writer *chunkWriter) Write(data []byte) (int, error) {
	for start := 0; start < len(data); start += 16 * 1024 {
		end := min(start+16*1024, len(data))
		if err := writer.send(wireMessage{Type: "output", ID: writer.jobID, Output: string(data[start:end])}); err != nil {
			return start, err
		}
	}
	return len(data), nil
}

func runJob(ctx context.Context, send func(wireMessage) error, message wireMessage) {
	if err := send(wireMessage{Type: "started", ID: message.ID}); err != nil {
		return
	}
	cmd := exec.CommandContext(ctx, "sh", "-lc", message.Command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = time.Second
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	writer := &chunkWriter{send: send, jobID: message.ID}
	cmd.Stdout, cmd.Stderr = writer, writer
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitCode = -1
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		}
	}
	_ = send(wireMessage{Type: "result", ID: message.ID, ExitCode: exitCode})
}
