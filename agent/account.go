package main

import (
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

func accountRequest(server, token, method, path string) (*http.Response, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("API token required; pass --token or set RELAY_API_TOKEN")
	}
	req, err := http.NewRequest(method, strings.TrimRight(server, "/")+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
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
