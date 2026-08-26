package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type accountDevice struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Workspace          string   `json:"workspace_name"`
	AgentVersion       string   `json:"agent_version"`
	Capabilities       []string `json:"capabilities"`
	Online             bool     `json:"online"`
	IdentityPublicKey  string   `json:"identity_public_key"`
	TransportPublicKey string   `json:"transport_public_key"`
}

func (device accountDevice) supports(capability string) bool {
	for _, value := range device.Capabilities {
		if value == capability {
			return true
		}
	}
	return false
}

type accountProcess struct {
	ID       string `json:"id"`
	DeviceID string `json:"device_id"`
	Status   string `json:"status"`
	Output   string `json:"output"`
	Revision int    `json:"revision"`
	ExitCode *int   `json:"exit_code"`
	Signal   string `json:"signal"`
}

type accountAction struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Workspace string `json:"workspace_name"`
	Command   string `json:"command"`
	Cwd       string `json:"cwd"`
	Confirm   int    `json:"confirm"`
}

func accountRequest(server, token, method, path string) (*http.Response, error) {
	return accountJSONRequest(server, token, method, path, nil)
}

func accountJSONRequest(server, token, method, path string, body any) (*http.Response, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("RC credential required; sign in with rc login, or pass a PoP API key with --token / RC_API_TOKEN")
	}
	var reader io.Reader
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, strings.TrimRight(server, "/")+path, reader)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(token, "rcsk_") {
		if err := signAPIRequest(req, token, data); err != nil {
			return nil, err
		}
	} else {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return http.DefaultClient.Do(req)
}

func apiSigningKey(secret string) (string, ed25519.PrivateKey, error) {
	parts := strings.SplitN(strings.TrimPrefix(secret, "rcsk_"), "_", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "", nil, fmt.Errorf("invalid RC API signing key")
	}
	der, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", nil, fmt.Errorf("invalid RC API signing key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return "", nil, fmt.Errorf("invalid RC API signing key")
	}
	privateKey, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return "", nil, fmt.Errorf("invalid RC API signing key")
	}
	return parts[0], privateKey, nil
}

func signAPIRequest(req *http.Request, secret string, body []byte) error {
	keyID, privateKey, err := apiSigningKey(secret)
	if err != nil {
		return err
	}
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	nonce := randomURLBytes(18)
	digest := sha256.Sum256(body)
	payload := "rc-api-v1\n" + keyID + "\n" + timestamp + "\n" + nonce + "\n" + req.Method + "\n" + req.URL.RequestURI() + "\n" + hex.EncodeToString(digest[:])
	signature := ed25519.Sign(privateKey, []byte(payload))
	req.Header.Set("X-RC-Key-ID", keyID)
	req.Header.Set("X-RC-Timestamp", timestamp)
	req.Header.Set("X-RC-Nonce", nonce)
	req.Header.Set("X-RC-Signature", base64.RawURLEncoding.EncodeToString(signature))
	return nil
}

func listAccountDevices(server, token string) ([]accountDevice, error) {
	resp, err := accountRequest(server, token, http.MethodGet, "/api/v1/devices")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var out struct {
		Devices []accountDevice `json:"devices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Devices, nil
}

func deleteAccountDevice(server, token, deviceID string) error {
	resp, err := accountRequest(server, token, http.MethodDelete, "/api/v1/devices/"+deviceID)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
}

func fetchAccountDevice(server, token, deviceID string) (accountDevice, error) {
	resp, err := accountRequest(server, token, http.MethodGet, "/api/v1/devices/"+deviceID)
	if err != nil {
		return accountDevice{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return accountDevice{}, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var out struct {
		Device accountDevice `json:"device"`
	}
	err = json.NewDecoder(resp.Body).Decode(&out)
	return out.Device, err
}
