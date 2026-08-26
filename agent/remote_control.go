package main

import (
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

type remoteControl struct {
	deviceID  string
	sessionID string
	aead      cipher.AEAD
	sendSeq   uint64
	recvSeq   uint64
	transport controlTransport
}

func accountControlIdentity(server, token string) (string, ed25519.PrivateKey, error) {
	if strings.HasPrefix(token, "rcsk_") {
		return apiSigningKey(token)
	}
	account, err := loadAccountSession(resolveStateDir(""))
	if err != nil || account.Token != token || strings.TrimRight(account.Server, "/") != strings.TrimRight(server, "/") {
		return "", nil, errors.New("CLI control key unavailable; run rc login again")
	}
	privateKey, err := base64.RawURLEncoding.DecodeString(account.ControlPrivateKey)
	if err != nil || len(privateKey) != ed25519.PrivateKeySize || account.ControlClientID == "" {
		return "", nil, errors.New("CLI control key unavailable; run rc login again")
	}
	return account.ControlClientID, ed25519.PrivateKey(privateKey), nil
}

func websocketHeaders(server, token, rawURL string) (http.Header, error) {
	headers := http.Header{}
	if strings.HasPrefix(token, "rcsk_") {
		req, _ := http.NewRequest(http.MethodGet, rawURL, nil)
		if err := signAPIRequest(req, token, nil); err != nil {
			return nil, err
		}
		return req.Header, nil
	}
	headers.Set("Authorization", "Bearer "+token)
	return headers, nil
}

func writeJSON(conn *websocket.Conn, mu *sync.Mutex, value any) error {
	mu.Lock()
	defer mu.Unlock()
	return conn.WriteJSON(value)
}

func waitOuterResponse(conn *websocket.Conn, requestID string) (map[string]any, error) {
	for {
		var message map[string]any
		if err := conn.ReadJSON(&message); err != nil {
			return nil, err
		}
		if message["type"] != "response" || message["requestId"] != requestID {
			continue
		}
		if message["ok"] != true {
			return nil, fmt.Errorf("%v", message["error"])
		}
		result, _ := message["result"].(map[string]any)
		return result, nil
	}
}

func deviceIdentityKey(pemValue string) (ed25519.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemValue))
	if block == nil {
		return nil, errors.New("invalid RC Node identity")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	publicKey, ok := key.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("invalid RC Node identity")
	}
	return publicKey, nil
}

func openRemoteControl(server, token string, device accountDevice) (*remoteControl, error) {
	clientID, signingKey, err := accountControlIdentity(server, token)
	if err != nil {
		return nil, err
	}
	detail, err := fetchAccountDevice(server, token, device.ID)
	if err != nil {
		return nil, err
	}
	if detail.IdentityPublicKey == "" || detail.TransportPublicKey == "" {
		return nil, errors.New("RC Node requires an update before encrypted control")
	}
	if err := verifyDevicePin(resolveStateDir(""), detail); err != nil {
		return nil, err
	}
	u, _ := url.Parse(strings.TrimRight(server, "/") + "/api/v1/ws")
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	headers, err := websocketHeaders(server, token, u.String())
	if err != nil {
		return nil, err
	}
	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), headers)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("websocket: %s", resp.Status)
		}
		return nil, err
	}
	conn.SetReadLimit(maxWireMessageBytes)
	writeMu := sync.Mutex{}
	challengeID := randomURLBytes(12)
	if err := writeJSON(conn, &writeMu, map[string]any{"type": "control.challenge", "requestId": challengeID, "deviceId": device.ID}); err != nil {
		conn.Close()
		return nil, err
	}
	challengeResult, err := waitOuterResponse(conn, challengeID)
	if err != nil {
		conn.Close()
		return nil, err
	}
	challenge, _ := challengeResult["challenge"].(string)
	if challenge == "" {
		conn.Close()
		return nil, errors.New("missing Node challenge")
	}
	ephemeral, err := generateX25519()
	if err != nil {
		conn.Close()
		return nil, err
	}
	signature := ed25519.Sign(signingKey, []byte(sessionPayload(challenge, device.ID, clientID, ephemeral.Public)))
	openID := randomURLBytes(12)
	if err := writeJSON(conn, &writeMu, map[string]any{"type": "control.open", "requestId": openID, "deviceId": device.ID,
		"challenge": challenge, "clientId": clientID, "publicKey": ephemeral.Public, "signature": base64.RawURLEncoding.EncodeToString(signature)}); err != nil {
		conn.Close()
		return nil, err
	}
	ready, err := waitOuterResponse(conn, openID)
	if err != nil {
		conn.Close()
		return nil, err
	}
	sessionID, _ := ready["sessionId"].(string)
	transportKey, _ := ready["transportPublicKey"].(string)
	ephemeralKey, _ := ready["ephemeralPublicKey"].(string)
	deviceSig, _ := ready["signature"].(string)
	if sessionID == "" || ephemeralKey == "" || transportKey != detail.TransportPublicKey {
		conn.Close()
		return nil, errors.New("RC Node transport identity changed")
	}
	identityKey, err := deviceIdentityKey(detail.IdentityPublicKey)
	if err != nil {
		conn.Close()
		return nil, err
	}
	sigBytes, err := base64.RawURLEncoding.DecodeString(deviceSig)
	if err != nil || !ed25519.Verify(identityKey, []byte(readyPayload(challenge, device.ID, clientID, ephemeral.Public, transportKey, ephemeralKey, sessionID)), sigBytes) {
		conn.Close()
		return nil, errors.New("RC Node handshake signature failed")
	}
	aead, err := deriveClientAEAD(ephemeral.Private, transportKey, ephemeralKey, challenge, device.ID, clientID)
	if err != nil {
		conn.Close()
		return nil, err
	}
	fallback := &websocketControlTransport{conn: conn, writeMu: &writeMu}
	transport := controlTransport(fallback)
	if device.supports("webrtc") {
		if direct, directErr := openWebRTCClientTransport(fallback, device.ID, sessionID, decodeIceServers(ready["iceServers"])); directErr == nil {
			transport = direct
		}
	}
	control := &remoteControl{deviceID: device.ID, sessionID: sessionID, aead: aead, transport: transport}
	return control, nil
}

type x25519Pair struct{ Private, Public string }

func generateX25519() (x25519Pair, error) {
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return x25519Pair{}, err
	}
	return x25519Pair{Private: base64.RawURLEncoding.EncodeToString(privateKey.Bytes()), Public: base64.RawURLEncoding.EncodeToString(privateKey.PublicKey().Bytes())}, nil
}

func (control *remoteControl) send(message wireMessage) error {
	plaintext, _ := json.Marshal(message)
	if len(plaintext) > maxControlPlaintext {
		return errors.New("control message is too large")
	}
	control.sendSeq++
	sequence := control.sendSeq
	ciphertext := control.aead.Seal(nil, frameNonce(1, sequence), plaintext, frameAAD(control.sessionID, sequence, "c2n"))
	return control.transport.sendFrame(control.deviceID, control.sessionID, sequence, base64.RawURLEncoding.EncodeToString(ciphertext))
}

func (control *remoteControl) read() (wireMessage, error) {
	next, encoded, err := control.transport.readFrame(control.sessionID)
	if err != nil {
		return wireMessage{}, err
	}
	if next != control.recvSeq+1 {
		return wireMessage{}, errors.New("invalid encrypted frame sequence")
	}
	if len(encoded) == 0 || len(encoded) > maxControlCiphertext {
		return wireMessage{}, errors.New("invalid encrypted control frame")
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return wireMessage{}, err
	}
	plaintext, err := control.aead.Open(nil, frameNonce(2, next), ciphertext, frameAAD(control.sessionID, next, "n2c"))
	if err != nil {
		return wireMessage{}, errors.New("encrypted frame authentication failed")
	}
	control.recvSeq = next
	var message wireMessage
	if err := json.Unmarshal(plaintext, &message); err != nil {
		return wireMessage{}, err
	}
	if message.Type == "control.revoked" {
		return wireMessage{}, errors.New("control authorization changed; reopen the session")
	}
	return message, nil
}

func (control *remoteControl) request(message wireMessage) error {
	message.RequestID = randomURLBytes(12)
	if err := control.send(message); err != nil {
		return err
	}
	for {
		response, err := control.read()
		if err != nil {
			return err
		}
		if response.Type == "control.result" && response.RequestID == message.RequestID {
			if response.Output == "ok" {
				return nil
			}
			return errors.New(response.Output)
		}
	}
}

func (control *remoteControl) close() { _ = control.transport.close() }

func waitForProcess(control *remoteControl, processID string) error {
	for {
		message, err := control.read()
		if err != nil {
			return err
		}
		if message.ID != processID {
			continue
		}
		if message.Type == "process.output" {
			fmt.Print(message.Output)
		}
		if message.Type == "process.exit" {
			if message.ExitCode != nil && *message.ExitCode != 0 {
				return fmt.Errorf("process exited %d", *message.ExitCode)
			}
			return nil
		}
	}
}
