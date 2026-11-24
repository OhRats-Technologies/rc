package node

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
	"github.com/OhRats-Technologies/rc/pkg/version"
	"golang.org/x/net/websocket"
)

// RunControl keeps one authenticated Node command channel open, reconnecting
// after transport failures until ctx ends. Execution state remains Node-local.
func RunControl(ctx context.Context, server, stateDir string, state *NodeState, exec ExecutionManager, logf func(string), control ...*ControlManager) {
	if logf == nil {
		logf = func(string) {}
	}
	hosted := NewHostedMCP(stateDir, state, exec)
	var sessions *ControlManager
	if len(control) > 0 {
		sessions = control[0]
	}
	backoff := time.Second
	for {
		started := time.Now()
		err := runControlOnce(ctx, server, stateDir, state, exec, hosted, sessions, logf)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			logf(fmt.Sprintf("control channel closed after %s: %v", time.Since(started).Round(time.Second), err))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 15*time.Second {
			backoff *= 2
			if backoff > 15*time.Second {
				backoff = 15 * time.Second
			}
		}
	}
}

func runControlOnce(ctx context.Context, server, stateDir string, state *NodeState, exec ExecutionManager, hosted *HostedMCP, control *ControlManager, logf func(string)) error {
	target, err := controlURL(server)
	if err != nil {
		return err
	}
	auth, err := SignNodeRequest(state, http.MethodGet, "/api/v1/node/connect", nil)
	if err != nil {
		return err
	}
	headers := http.Header{}
	for key, value := range auth.Headers() {
		headers.Set(key, value)
	}
	originScheme := "http"
	if target.Scheme == "wss" {
		originScheme = "https"
	}
	origin := &url.URL{Scheme: originScheme, Host: target.Host}
	conn, err := websocket.DialConfig(&websocket.Config{
		Location: target,
		Origin:   origin,
		Version:  websocket.ProtocolVersionHybi13,
		Header:   headers,
	})
	if err != nil {
		return err
	}
	defer conn.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	var sendMu sync.Mutex
	send := func(message protocol.NodeToServer) error {
		encoded, err := json.Marshal(message)
		if err != nil {
			return err
		}
		frames, err := protocol.Encode(string(encoded))
		if err != nil {
			return err
		}
		sendMu.Lock()
		defer sendMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
		for _, frame := range frames {
			if err := websocket.Message.Send(conn, frame); err != nil {
				return err
			}
		}
		return nil
	}
	hosted.SetSend(send)
	if control != nil {
		control.SetProcessStateHandler(func(message protocol.NodeToServer) { _ = send(message) })
	}
	transportKey, _ := state.TransportPublicKey()
	lockHash, lockGeneration := LockMetadata(stateDir)
	_ = send(protocol.NodeToServer{
		Type: "hello",
		Hello: &protocol.NodeHello{
			Version:            version.Version,
			Hostname:           getHostname(),
			Platform:           getPlatform(),
			Arch:               getArch(),
			Capabilities:       NodeCapabilities,
			TransportPublicKey: transportKey,
			LockHash:           lockHash,
			LockGeneration:     lockGeneration,
		},
	})
	_ = send(protocol.NodeToServer{Type: "process_sync", IDs: exec.ActiveIDs()})
	logf("control channel connected")
	// Idle WebSockets are closed by proxies; keepalives keep the channel open between commands.
	go func() {
		ticker := time.NewTicker(protocol.KeepaliveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if send(protocol.NodeToServer{Type: protocol.TypeKeepalive}) != nil {
					_ = conn.Close()
					return
				}
			}
		}
	}()
	decoder := &protocol.Decoder{}
	serverKeepalive := false
	for {
		if serverKeepalive {
			_ = conn.SetReadDeadline(time.Now().Add(protocol.IdleTimeout))
		}
		var payload []byte
		if err := websocket.Message.Receive(conn, &payload); err != nil {
			return err
		}
		plain, done, err := decoder.Accept(payload)
		if err != nil || !done {
			continue
		}
		var message protocol.ServerToNode
		if json.Unmarshal(plain, &message) != nil {
			continue
		}
		switch message.Type {
		case protocol.TypeKeepalive:
			serverKeepalive = true
		case "lock_bootstrap":
			if message.Snapshot != "" {
				if err := BootstrapLock(stateDir, message.Snapshot, server); err != nil {
					logf("lock bootstrap: " + err.Error())
				} else {
					logf("RC Lock bootstrapped")
					if snapshot, err := ValidateSnapshot(message.Snapshot); err == nil {
						hosted.AuthorityChanged(snapshot)
					}
					if control != nil {
						control.CloseAllSessions()
					}
				}
				hash, generation := LockMetadata(stateDir)
				_ = send(protocol.NodeToServer{Type: "lock_state", Hash: hash, Generation: generation})
			}
		case "lock_sync":
			proof := &protocol.ControlProof{
				Grant: message.Grant, CredentialID: message.CredentialID,
				Assertion: message.Assertion,
			}
			if err := SyncLock(stateDir, message.Snapshot, message.PreviousHash,
				message.PreviousGeneration, proof, message.Signature); err != nil {
				logf("lock sync: " + err.Error())
			} else {
				logf("RC Lock sync applied")
				if snapshot, err := ValidateSnapshot(message.Snapshot); err == nil {
					hosted.AuthorityChanged(snapshot)
				}
				if control != nil {
					control.CloseAllSessions()
				}
			}
			hash, generation := LockMetadata(stateDir)
			_ = send(protocol.NodeToServer{Type: "lock_state", Hash: hash, Generation: generation})
		case protocol.TypeControlChallenge:
			if control == nil {
				sendControlError(send, message.RequestID, "control manager unavailable")
				continue
			}
			send(protocol.NodeToServer{
				Type: protocol.TypeControlChallenge, RequestID: message.RequestID,
				Challenge: control.CreateChallenge(message.RequestID),
			})
		case protocol.TypeControlOpen:
			if control == nil {
				sendControlError(send, message.RequestID, "control manager unavailable")
				continue
			}
			authority, err := controlSessionAuthority(stateDir, message)
			if err != nil {
				sendControlError(send, message.RequestID, err.Error())
				continue
			}
			ready, err := control.OpenAuthorizedSession(
				message.Challenge, message.ClientID, message.PublicKey,
				message.Signature, authority, send,
			)
			if err != nil {
				sendControlError(send, message.RequestID, err.Error())
				continue
			}
			send(protocol.NodeToServer{
				Type: protocol.TypeControlOpen, RequestID: message.RequestID,
				SessionID: ready.SessionID, TransportPublicKey: ready.TransportPublicKey,
				EphemeralPublicKey: ready.EphemeralPublicKey, Signature: ready.Signature,
			})
		case protocol.TypeControlFrame:
			if control == nil {
				continue
			}
			if _, err := control.HandleDirectFrame(message.SessionID, message.Sequence, message.Ciphertext); err != nil {
				logf("control frame: " + err.Error())
			}
		case protocol.TypeControlClose:
			if control != nil {
				control.CloseSession(message.SessionID)
			}
		case "mcp_start", "mcp_status", "mcp_input", "mcp_cancel", "mcp_image":
			go hosted.Handle(message)
		}
	}
}

func controlURL(server string) (*url.URL, error) {
	target, err := url.Parse(server)
	if err != nil || target.Host == "" {
		return nil, err
	}
	switch target.Scheme {
	case "https":
		target.Scheme = "wss"
	case "http":
		target.Scheme = "ws"
	default:
		return nil, &url.Error{Op: "control", URL: server, Err: errInvalidControlURL{}}
	}
	target.Path = "/api/v1/node/connect"
	target.RawQuery = ""
	return target, nil
}

type errInvalidControlURL struct{}

func (errInvalidControlURL) Error() string { return "RC server URL must use http or https" }

func sendControlError(send func(protocol.NodeToServer) error, requestID, message string) {
	_ = send(protocol.NodeToServer{
		Type: protocol.TypeControlError, RequestID: requestID, Error: message,
	})
}
