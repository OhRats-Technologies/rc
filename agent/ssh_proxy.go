package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

func sshProxyCommand(args []string) error {
	flags := flag.NewFlagSet("rc ssh-proxy", flag.ContinueOnError)
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
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: rc ssh-proxy [--url URL]")
	}

	u, err := url.Parse(strings.TrimRight(*server, "/"))
	if err != nil || u.Host == "" {
		return errors.New("invalid RC server URL")
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	default:
		return errors.New("RC server URL must use http or https")
	}
	u.Path = "/api/v1/ssh/tunnel"
	u.RawQuery = ""
	conn, response, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		if response != nil {
			return fmt.Errorf("SSH tunnel: %s", response.Status)
		}
		return fmt.Errorf("SSH tunnel: %w", err)
	}
	defer conn.Close()
	conn.SetReadLimit(2 * 1024 * 1024)

	var writeMu sync.Mutex
	inputDone := make(chan error, 1)
	go func() {
		buffer := make([]byte, 32*1024)
		for {
			n, readErr := os.Stdin.Read(buffer)
			if n > 0 {
				writeMu.Lock()
				err := conn.WriteMessage(websocket.BinaryMessage, buffer[:n])
				writeMu.Unlock()
				if err != nil {
					inputDone <- err
					return
				}
			}
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					writeMu.Lock()
					_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "EOF"), time.Now().Add(time.Second))
					writeMu.Unlock()
					inputDone <- nil
					return
				}
				inputDone <- readErr
				return
			}
		}
	}()

	for {
		kind, data, readErr := conn.ReadMessage()
		if readErr != nil {
			select {
			case inputErr := <-inputDone:
				if inputErr != nil {
					return inputErr
				}
			default:
			}
			if websocket.IsCloseError(readErr, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return nil
			}
			return nil
		}
		if kind != websocket.BinaryMessage {
			continue
		}
		if _, err := os.Stdout.Write(data); err != nil {
			return err
		}
	}
}
