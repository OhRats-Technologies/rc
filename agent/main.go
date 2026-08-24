package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const version = "0.1.0"

type state struct {
	DeviceID   string `json:"deviceId"`
	PrivateKey string `json:"privateKey"`
}

type enrollRequest struct {
	Token        string   `json:"token"`
	Name         string   `json:"name"`
	Hostname     string   `json:"hostname"`
	Platform     string   `json:"platform"`
	Arch         string   `json:"arch"`
	PublicKey    string   `json:"publicKey"`
	AgentVersion string   `json:"agentVersion"`
	Capabilities []string `json:"capabilities"`
}

type enrollResponse struct {
	DeviceID string `json:"deviceId"`
}

type wireMessage struct {
	Type     string `json:"type"`
	ID       string `json:"id,omitempty"`
	Command  string `json:"command,omitempty"`
	Output   string `json:"output,omitempty"`
	ExitCode int    `json:"exitCode"`
}

func main() {
	defaultURL := env("RELAY_URL", "https://relay.ohrats.party")
	defaultToken := os.Getenv("RELAY_ENROLL_TOKEN")
	defaultState := env("RELAY_STATE_DIR", defaultStateDir())

	serverURL := flag.String("url", defaultURL, "Relay server URL")
	enrollToken := flag.String("enroll", defaultToken, "Fleet enrollment token")
	stateDir := flag.String("state-dir", defaultState, "Agent state directory")
	name := flag.String("name", "", "Device display name")
	flag.Parse()

	if err := os.MkdirAll(*stateDir, 0700); err != nil {
		log.Fatal(err)
	}

	st, err := loadState(*stateDir)
	if errors.Is(err, os.ErrNotExist) {
		if strings.TrimSpace(*enrollToken) == "" {
			log.Fatal("device is not enrolled; pass --enroll TOKEN or RELAY_ENROLL_TOKEN")
		}
		st, err = enroll(*serverURL, *enrollToken, *name)
		if err != nil {
			log.Fatalf("enroll: %v", err)
		}
		if err := saveState(*stateDir, st); err != nil {
			log.Fatalf("save state: %v", err)
		}
		log.Printf("enrolled device %s", st.DeviceID)
	} else if err != nil {
		log.Fatal(err)
	}

	ctxDone := make(chan os.Signal, 1)
	signal.Notify(ctxDone, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-ctxDone:
			return
		default:
		}
		if err := connect(*serverURL, st); err != nil {
			log.Printf("connection ended: %v", err)
		}
		time.Sleep(3 * time.Second)
	}
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func defaultStateDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".relay"
	}
	return filepath.Join(home, ".config", "relay")
}

func statePath(dir string) string { return filepath.Join(dir, "device.json") }

func loadState(dir string) (state, error) {
	var st state
	b, err := os.ReadFile(statePath(dir))
	if err != nil {
		return st, err
	}
	err = json.Unmarshal(b, &st)
	return st, err
}

func saveState(dir string, st state) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(dir), b, 0600)
}

func enroll(serverURL, token, displayName string) (state, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return state{}, err
	}
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return state{}, err
	}
	pubPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	hostname, _ := os.Hostname()
	if displayName == "" {
		displayName = hostname
	}
	payload := enrollRequest{
		Token: token, Name: displayName, Hostname: hostname,
		Platform: runtime.GOOS, Arch: runtime.GOARCH, PublicKey: pubPEM,
		AgentVersion: version, Capabilities: []string{"shell"},
	}
	b, _ := json.Marshal(payload)
	endpoint := strings.TrimRight(serverURL, "/") + "/api/v1/agent/enroll"
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(b))
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

func connect(serverURL string, st state) error {
	privBytes, err := base64.RawStdEncoding.DecodeString(st.PrivateKey)
	if err != nil || len(privBytes) != ed25519.PrivateKeySize {
		return errors.New("invalid stored private key")
	}
	priv := ed25519.PrivateKey(privBytes)
	ts := fmt.Sprintf("%d", time.Now().Unix())
	message := "relay:" + st.DeviceID + ":" + ts
	sig := base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, []byte(message)))

	u, err := url.Parse(strings.TrimRight(serverURL, "/"))
	if err != nil {
		return err
	}
	if u.Scheme == "https" { u.Scheme = "wss" } else { u.Scheme = "ws" }
	u.Path = "/api/v1/agent/ws"
	q := u.Query()
	q.Set("device", st.DeviceID)
	q.Set("ts", ts)
	q.Set("sig", sig)
	u.RawQuery = q.Encode()

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	log.Printf("connected as %s", st.DeviceID)
	var writeMu sync.Mutex

	done := make(chan error, 1)
	jobs := make(chan wireMessage, 32)
	go func() {
		for msg := range jobs {
			runJob(conn, &writeMu, msg)
		}
	}()
	go func() {
		defer close(jobs)
		for {
			var msg wireMessage
			if err := conn.ReadJSON(&msg); err != nil {
				done <- err
				return
			}
			if msg.Type != "job" || msg.ID == "" {
				continue
			}
			jobs <- msg
		}
	}()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			return err
		case <-ticker.C:
			writeMu.Lock()
			err := conn.WriteJSON(wireMessage{Type: "heartbeat"})
			writeMu.Unlock()
			if err != nil {
				return err
			}
		}
	}
}

func runJob(conn *websocket.Conn, writeMu *sync.Mutex, msg wireMessage) {
	cmd := exec.Command("sh", "-lc", `( eval "$RELAY_COMMAND" ); code=$?; printf '\n__RELAY_EXIT__%d\n' "$code"`)
	cmd.Env = append(os.Environ(), "RELAY_COMMAND="+msg.Command)
	output, err := cmd.CombinedOutput()
	exitCode := -1
	marker := []byte("\n__RELAY_EXIT__")
	if at := bytes.LastIndex(output, marker); at >= 0 {
		var parsed int
		if _, scanErr := fmt.Sscanf(string(output[at+len(marker):]), "%d", &parsed); scanErr == nil {
			exitCode = parsed
			output = output[:at]
		}
	}
	if exitCode < 0 && err == nil {
		exitCode = 0
	}
	if exitCode < 0 && len(output) == 0 && err != nil {
		output = append(output, []byte(err.Error())...)
	}
	if len(output) > 1024*1024 {
		output = output[:1024*1024]
	}
	writeMu.Lock()
	_ = conn.WriteJSON(wireMessage{Type: "result", ID: msg.ID, Output: string(output), ExitCode: exitCode})
	writeMu.Unlock()
}

