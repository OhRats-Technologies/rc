package main

import (
	"bufio"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

func shellCommand(args []string) error {
	flags, server, token, err := accountFlags("ohrats-rc shell", args)
	if err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: ohrats-rc shell [--token TOKEN] DEVICE")
	}
	device, err := resolveAccountDevice(*server, *token, flags.Arg(0))
	if err != nil {
		return err
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("shell requires an interactive terminal")
	}

	u, _ := url.Parse(strings.TrimRight(*server, "/") + "/api/v1/ws")
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	headers := http.Header{"Authorization": []string{"Bearer " + *token}}
	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), headers)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("websocket: %s", resp.Status)
		}
		return err
	}
	defer conn.Close()
	var writeMu sync.Mutex
	send := func(value any) error { writeMu.Lock(); defer writeMu.Unlock(); return conn.WriteJSON(value) }

	old, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	defer term.Restore(int(os.Stdin.Fd()), old)
	cols, rows, _ := term.GetSize(int(os.Stdin.Fd()))
	if cols < 2 {
		cols = 80
	}
	if rows < 2 {
		rows = 24
	}
	requestID := fmt.Sprintf("cli-%d", time.Now().UnixNano())
	if err := send(map[string]any{"type": "process.start", "requestId": requestID, "deviceId": device.ID, "command": `exec "${SHELL:-sh}" -l`, "cols": cols, "rows": rows}); err != nil {
		return err
	}

	processID := ""
	var processMu sync.RWMutex
	setProcess := func(value string) { processMu.Lock(); processID = value; processMu.Unlock() }
	getProcess := func() string { processMu.RLock(); defer processMu.RUnlock(); return processID }
	done := make(chan error, 1)
	go readShellEvents(conn, requestID, *server, *token, getProcess, setProcess, done)
	go forwardShellInput(send, getProcess)

	resize := make(chan os.Signal, 1)
	signal.Notify(resize, syscall.SIGWINCH)
	defer signal.Stop(resize)
	for {
		select {
		case err := <-done:
			return err
		case <-resize:
			if id := getProcess(); id != "" {
				cols, rows, _ := term.GetSize(int(os.Stdin.Fd()))
				_ = send(map[string]any{"type": "process.resize", "processId": id, "cols": cols, "rows": rows})
			}
		}
	}
}

func readShellEvents(conn *websocket.Conn, requestID, server, token string, getProcess func() string, setProcess func(string), done chan<- error) {
	revision := 0
	for {
		var message map[string]any
		if err := conn.ReadJSON(&message); err != nil {
			done <- err
			return
		}
		if message["type"] == "response" && message["requestId"] == requestID {
			if message["ok"] != true {
				done <- fmt.Errorf("%v", message["error"])
				return
			}
			if result, ok := message["result"].(map[string]any); ok {
				if id, ok := result["processId"].(string); ok {
					setProcess(id)
					if process, err := fetchAccountProcess(server, token, id); err == nil {
						fmt.Print(process.Output)
						revision = process.Revision
					}
				}
			}
			continue
		}
		if message["type"] != "event" {
			continue
		}
		event, _ := message["event"].(map[string]any)
		if event == nil || event["processId"] != getProcess() {
			continue
		}
		kind, _ := event["kind"].(string)
		detail, _ := event["detail"].(map[string]any)
		if kind == "process.output" {
			next, _ := detail["revision"].(float64)
			if int(next) <= revision {
				continue
			}
			if chunk, ok := detail["chunk"].(string); ok {
				fmt.Print(chunk)
				revision = int(next)
			}
		}
		if kind == "process.exited" || kind == "process.lost" {
			done <- nil
			return
		}
	}
}

func forwardShellInput(send func(any) error, getProcess func() string) {
	reader := bufio.NewReader(os.Stdin)
	buffer := make([]byte, 4096)
	for {
		n, err := reader.Read(buffer)
		id := getProcess()
		if n > 0 && id != "" {
			_ = send(map[string]any{"type": "process.input", "processId": id, "data": string(buffer[:n])})
		}
		if err != nil {
			return
		}
	}
}
