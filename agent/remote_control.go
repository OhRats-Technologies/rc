package main

import (
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type remoteControl struct {
	deviceID  string
	sessionID string
	server    string
	token     string
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

func controlJSON(server, token, method, path string, body, out any) error {
	resp, err := accountJSONRequest(server, token, method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var payload struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &payload)
		if payload.Error != "" {
			return errors.New(payload.Error)
		}
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
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
	var challengeResult struct {
		Challenge string `json:"challenge"`
	}
	if err := controlJSON(server, token, http.MethodPost, "/api/v1/control/challenge", map[string]any{"deviceId": device.ID}, &challengeResult); err != nil {
		return nil, err
	}
	if challengeResult.Challenge == "" {
		return nil, errors.New("missing Node challenge")
	}
	ephemeral, err := generateX25519()
	if err != nil {
		return nil, err
	}
	signature := ed25519.Sign(signingKey, []byte(sessionPayload(challengeResult.Challenge, device.ID, clientID, ephemeral.Public)))
	var ready struct {
		SessionID          string      `json:"sessionId"`
		TransportPublicKey string      `json:"transportPublicKey"`
		EphemeralPublicKey string      `json:"ephemeralPublicKey"`
		Signature          string      `json:"signature"`
		IceServers         []iceServer `json:"iceServers"`
	}
	if err := controlJSON(server, token, http.MethodPost, "/api/v1/control/open", map[string]any{
		"deviceId": device.ID, "challenge": challengeResult.Challenge, "clientId": clientID,
		"publicKey": ephemeral.Public, "signature": base64.RawURLEncoding.EncodeToString(signature),
	}, &ready); err != nil {
		return nil, err
	}
	if ready.SessionID == "" || ready.EphemeralPublicKey == "" || ready.TransportPublicKey != detail.TransportPublicKey {
		return nil, errors.New("RC Node transport identity changed")
	}
	identityKey, err := deviceIdentityKey(detail.IdentityPublicKey)
	if err != nil {
		return nil, err
	}
	sigBytes, err := base64.RawURLEncoding.DecodeString(ready.Signature)
	if err != nil || !ed25519.Verify(identityKey, []byte(readyPayload(challengeResult.Challenge, device.ID, clientID, ephemeral.Public,
		ready.TransportPublicKey, ready.EphemeralPublicKey, ready.SessionID)), sigBytes) {
		return nil, errors.New("RC Node handshake signature failed")
	}
	aead, err := deriveClientAEAD(ephemeral.Private, ready.TransportPublicKey, ready.EphemeralPublicKey, challengeResult.Challenge, device.ID, clientID)
	if err != nil {
		return nil, err
	}
	if !device.supports("webrtc") {
		return nil, errors.New("RC Node does not support WebRTC control")
	}
	transport, directErr := openWebRTCClientTransport(server, token, device.ID, ready.SessionID, ready.IceServers)
	if directErr != nil {
		_ = controlJSON(server, token, http.MethodDelete, "/api/v1/control/"+url.PathEscape(ready.SessionID), nil, nil)
		return nil, fmt.Errorf("WebRTC control unavailable: %w", directErr)
	}
	return &remoteControl{deviceID: device.ID, sessionID: ready.SessionID, server: server, token: token, aead: aead, transport: transport}, nil
}
