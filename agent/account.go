package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type accountDevice struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Workspace    string `json:"workspace_name"`
	AgentVersion string `json:"agent_version"`
	Online       bool   `json:"online"`
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
}

func accountRequest(server, token, method, path string) (*http.Response, error) {
	return accountJSONRequest(server, token, method, path, nil)
}

func accountJSONRequest(server, token, method, path string, body any) (*http.Response, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("API token required; pass --token or set RC_API_TOKEN")
	}
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, strings.TrimRight(server, "/")+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return http.DefaultClient.Do(req)
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
