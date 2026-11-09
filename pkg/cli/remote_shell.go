package cli

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/apiclient"
	"github.com/OhRats-Technologies/rc/pkg/crypto"
	"github.com/OhRats-Technologies/rc/pkg/protocol"
	"github.com/OhRats-Technologies/rc/pkg/transport"
	"github.com/pion/webrtc/v4"
)

func runRemoteShell(ctx context.Context, dir, server, deviceID, command string) error {
	credential, err := deviceCredential(dir, "")
	if err != nil {
		return err
	}
	if credential.Kind != apiclient.CredentialPop || credential.Pop == nil {
		return fmt.Errorf("remote shell requires rc login with an API key")
	}
	client, err := apiclient.New(server, credential)
	if err != nil {
		return err
	}
	device, err := client.Device(ctx, deviceID)
	if err != nil {
		return err
	}
	if !device.Supports("webrtc") {
		return fmt.Errorf("target Node does not support WebRTC control; run rc upgrade")
	}
	challenge, err := client.ControlChallenge(ctx, deviceID)
	if err != nil {
		return err
	}
	clientPrivate := randomURLBytes(32)
	clientPublic, err := crypto.X25519Public(clientPrivate)
	if err != nil {
		return err
	}
	payload := crypto.SessionPayload(challenge, deviceID, credential.Pop.ID, clientPublic)
	signature := base64.RawURLEncoding.EncodeToString(ed25519.Sign(credential.Pop.Signing, []byte(payload)))
	ready, err := client.ControlOpen(ctx, apiclient.ControlOpen{
		DeviceID: deviceID, Challenge: challenge, ClientID: credential.Pop.ID,
		PublicKey: clientPublic, Signature: signature,
	})
	if err != nil {
		return err
	}
	readyPayload := crypto.ReadyPayload(challenge, deviceID, credential.Pop.ID, clientPublic,
		ready.TransportPublicKey, ready.EphemeralPublicKey, ready.SessionID,
		protocol.ControlAttemptsPayload(ready.Attempts))
	if err := crypto.VerifyEd25519(device.IdentityPublicKey, []byte(readyPayload), ready.Signature); err != nil {
		return fmt.Errorf("RC Node handshake signature failed")
	}
	key, err := crypto.DeriveClientKey(clientPrivate, ready.TransportPublicKey,
		ready.EphemeralPublicKey, challenge, deviceID, credential.Pop.ID)
	if err != nil {
		return err
	}
	peer, err := transport.NewPeer(iceServers(ready.IceServers))
	if err != nil {
		return err
	}
	defer peer.Close()
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.CloseControl(closeCtx, ready.SessionID)
	}()
	offer, err := peer.Offer()
	if err != nil {
		return err
	}
	answer, err := client.ControlWebRTC(ctx, ready.SessionID, deviceID, offer, protocol.ControlIceModeHost)
	if err != nil {
		return err
	}
	if err := peer.AcceptAnswer(answer); err != nil {
		return err
	}
	if err := peer.WaitOpen(15 * time.Second); err != nil {
		return err
	}
	processID := fmt.Sprintf("cli_%d", time.Now().UnixNano())
	mode := protocol.ExecutionMode{Kind: protocol.ExecutionModeSystemLoginShell}
	if command != "" {
		mode = protocol.ExecutionMode{Kind: protocol.ExecutionModeSystemShell, Command: command}
	}
	if err := sendControlFrame(peer, &key, ready.SessionID, 1, protocol.ControlMessage{
		Type: protocol.TypeProcessStart, ID: processID, Mode: &mode,
		Environment: &protocol.EnvironmentSpec{Base: protocol.EnvironmentBaseInherit},
		Terminal:    &protocol.TerminalSpec{Cols: 80, Rows: 24, Term: "xterm-256color"},
	}); err != nil {
		return err
	}
	if command == "" {
		go streamLocalInput(ctx, peer, &key, ready.SessionID, processID)
	}
	return readControlOutput(ctx, peer, &key, ready.SessionID)
}

func streamLocalInput(ctx context.Context, peer *transport.Peer, key *[32]byte, sessionID, processID string) {
	buf := make([]byte, 4096)
	sequence := uint64(1)
	var mu sync.Mutex
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			mu.Lock()
			sequence++
			_ = sendControlFrame(peer, key, sessionID, sequence, protocol.ControlMessage{
				Type: protocol.TypeProcessStdin, ID: processID,
				Data: base64.RawURLEncoding.EncodeToString(buf[:n]),
			})
			mu.Unlock()
		}
		if err != nil {
			mu.Lock()
			sequence++
			_ = sendControlFrame(peer, key, sessionID, sequence, protocol.ControlMessage{
				Type: protocol.TypeProcessStdinClose, ID: processID,
			})
			mu.Unlock()
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func readControlOutput(ctx context.Context, peer *transport.Peer, key *[32]byte, sessionID string) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		message, err := peer.Receive(time.Second)
		if err != nil {
			if strings.Contains(err.Error(), "closed") {
				return err
			}
			continue
		}
		var frame protocol.ControlTransportMessage
		if json.Unmarshal(message, &frame) != nil || frame.Type != protocol.TypeControlFrame ||
			frame.SessionID != sessionID {
			return fmt.Errorf("invalid control frame")
		}
		plain, err := crypto.DecryptFrame(key, 2, frame.Sequence, sessionID, "n2c", frame.Ciphertext)
		if err != nil {
			return err
		}
		var control protocol.ControlMessage
		if json.Unmarshal(plain, &control) != nil {
			return fmt.Errorf("invalid control message")
		}
		switch control.Type {
		case protocol.TypeProcessStdout:
			data, err := base64.RawURLEncoding.DecodeString(control.Data)
			if err != nil {
				return err
			}
			_, _ = os.Stdout.Write(data)
		case protocol.TypeProcessStderr:
			data, err := base64.RawURLEncoding.DecodeString(control.Data)
			if err != nil {
				return err
			}
			_, _ = os.Stderr.Write(data)
		case protocol.TypeProcessExit:
			if control.ExitCode != nil && *control.ExitCode != 0 {
				return fmt.Errorf("remote shell exited with status %d", *control.ExitCode)
			}
			return nil
		case protocol.TypeProcessStarted:
		}
	}
}

func sendControlFrame(peer *transport.Peer, key *[32]byte, sessionID string, sequence uint64,
	message protocol.ControlMessage,
) error {
	plain, err := json.Marshal(message)
	if err != nil {
		return err
	}
	ciphertext, err := crypto.EncryptFrame(key, 1, sequence, sessionID, "c2n", plain)
	if err != nil {
		return err
	}
	frame, err := json.Marshal(protocol.ControlTransportMessage{
		Type: protocol.TypeControlFrame, SessionID: sessionID,
		Sequence: sequence, Ciphertext: ciphertext,
	})
	if err != nil {
		return err
	}
	return peer.Send(frame)
}

func randomURLBytes(size int) string {
	value := make([]byte, size)
	_, _ = rand.Read(value)
	return base64.RawURLEncoding.EncodeToString(value)
}

func iceServers(values []protocol.IceServer) []webrtc.ICEServer {
	servers := make([]webrtc.ICEServer, 0, len(values))
	for _, value := range values {
		servers = append(servers, webrtc.ICEServer{
			URLs: value.URLs, Username: value.Username, Credential: value.Credential,
		})
	}
	return servers
}
