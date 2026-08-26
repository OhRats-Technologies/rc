package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type cliAuthorizationStart struct {
	RequestID       string `json:"requestId"`
	DeviceCode      string `json:"deviceCode"`
	VerificationURL string `json:"verificationUrl"`
	ExpiresAt       int64  `json:"expiresAt"`
	Interval        int    `json:"interval"`
}

type cliAuthorizationPoll struct {
	Pending bool   `json:"pending"`
	Token   string `json:"token"`
	User    *struct {
		Name string `json:"name"`
	} `json:"user"`
}

func publicJSON(server, path string, input any, output any) error {
	data, _ := json.Marshal(input)
	resp, err := http.Post(strings.TrimRight(server, "/")+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(output)
}

func openBrowser(value string) error {
	if os.Getenv("RC_NO_BROWSER") != "" {
		return errors.New("browser opening disabled")
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", value)
	case "linux":
		command = exec.Command("xdg-open", value)
	default:
		return fmt.Errorf("open %s in your browser", value)
	}
	return command.Start()
}

func loginCommand(args []string) error {
	flags := flag.NewFlagSet("rc login", flag.ContinueOnError)
	dir := resolveStateDir("")
	config, _ := loadConfig(dir)
	account, _ := loadAccountSession(dir)
	serverDefault := strings.TrimSpace(os.Getenv("RC_URL"))
	if serverDefault == "" {
		serverDefault = account.Server
	}
	if serverDefault == "" {
		serverDefault = config.Server
	}
	if serverDefault == "" {
		serverDefault = defaultServer
	}
	server := flags.String("url", serverDefault, "RC server URL")
	expires := flags.String("expires", "never", "authorization lifetime: 1h, 1d, 7d, 30d, 90d, 180d, 1y, never")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: rc login [--url URL] [--expires DURATION]")
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	clientID := randomURLBytes(18)
	var start cliAuthorizationStart
	if err := publicJSON(*server, "/api/v1/auth/cli/start", map[string]any{
		"clientId": clientID, "signingPublicKey": base64.RawURLEncoding.EncodeToString(publicKey), "lifetime": *expires,
	}, &start); err != nil {
		return err
	}
	fmt.Printf("Open this URL to authorize RC CLI:\n%s\n", start.VerificationURL)
	if err := openBrowser(start.VerificationURL); err == nil {
		fmt.Println("Waiting for browser authorization…")
	}
	interval := time.Duration(start.Interval) * time.Second
	if interval < time.Second {
		interval = 2 * time.Second
	}
	deadline := time.UnixMilli(start.ExpiresAt)
	for time.Now().Before(deadline) {
		var poll cliAuthorizationPoll
		err := publicJSON(*server, "/api/v1/auth/cli/poll", map[string]any{"requestId": start.RequestID, "deviceCode": start.DeviceCode}, &poll)
		if err != nil {
			return err
		}
		if !poll.Pending && poll.Token != "" {
			name := ""
			if poll.User != nil {
				name = poll.User.Name
			}
			if err := saveAccountSession(dir, accountSession{Server: strings.TrimRight(*server, "/"), Token: poll.Token, User: name,
				ControlClientID: clientID, ControlPrivateKey: base64.RawURLEncoding.EncodeToString(privateKey)}); err != nil {
				return err
			}
			if name == "" {
				fmt.Println("RC CLI authorized")
			} else {
				fmt.Printf("Signed in as %s\n", name)
			}
			return nil
		}
		time.Sleep(interval)
	}
	return errors.New("CLI authorization expired")
}

func logoutCommand(args []string) error {
	flags := flag.NewFlagSet("rc logout", flag.ContinueOnError)
	dir := resolveStateDir("")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: rc logout")
	}
	account, err := loadAccountSession(dir)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println("RC CLI is not signed in")
		return nil
	}
	if err != nil {
		return err
	}
	req, _ := http.NewRequest(http.MethodDelete, strings.TrimRight(account.Server, "/")+"/api/v1/auth/cli/session", nil)
	req.Header.Set("Authorization", "Bearer "+account.Token)
	if resp, requestErr := http.DefaultClient.Do(req); requestErr == nil {
		resp.Body.Close()
	}
	if err := os.Remove(accountPath(dir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fmt.Println("RC CLI signed out")
	return nil
}
