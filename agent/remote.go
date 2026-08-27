package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func resolveAccountDevice(server, token, value string) (accountDevice, error) {
	devices, err := listAccountDevices(server, token)
	if err != nil {
		return accountDevice{}, err
	}
	want := strings.TrimSpace(value)
	var matches []accountDevice
	for _, device := range devices {
		if device.ID == want || strings.EqualFold(device.Name, want) || strings.HasPrefix(device.ID, want) {
			matches = append(matches, device)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) == 0 {
		return accountDevice{}, fmt.Errorf("device %q not found", value)
	}
	return accountDevice{}, fmt.Errorf("device %q is ambiguous", value)
}

func startAccountProcess(server, token, deviceID string, terminal bool) (string, error) {
	body := map[string]any{"terminal": terminal}
	resp, err := accountJSONRequest(server, token, http.MethodPost, "/api/v1/devices/"+url.PathEscape(deviceID)+"/processes", body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var out struct {
		ProcessID string `json:"processId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.ProcessID, nil
}

func remoteRunCommand(args []string) error {
	separator := -1
	for i, arg := range args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 1 || separator == len(args)-1 {
		return errors.New("usage: rc run [--url URL] [--token TOKEN] DEVICE -- COMMAND [ARG...]")
	}
	flags, server, token, err := accountFlags("rc run", args[:separator])
	if err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: rc run [flags] DEVICE -- COMMAND")
	}
	device, err := resolveAccountDevice(*server, *token, flags.Arg(0))
	if err != nil {
		return err
	}
	processID, err := startAccountProcess(*server, *token, device.ID, false)
	if err != nil {
		return err
	}
	control, err := openRemoteControl(*server, *token, device)
	if err != nil {
		return err
	}
	defer control.close()
	if err := control.send(wireMessage{Type: "process.start", ID: processID,
		Command: strings.Join(args[separator+1:], " ")}); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Started %s on %s\n", processID, device.Name)
	return waitForProcess(control, processID)
}
