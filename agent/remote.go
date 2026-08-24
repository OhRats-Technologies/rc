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
	"time"
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

func startAccountProcess(server, token, deviceID, command, cwd string) (string, error) {
	body := map[string]any{"command": command, "cols": 80, "rows": 24}
	if cwd != "" {
		body["cwd"] = cwd
	}
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

func fetchAccountProcess(server, token, processID string) (accountProcess, error) {
	resp, err := accountRequest(server, token, http.MethodGet, "/api/v1/processes/"+url.PathEscape(processID))
	if err != nil {
		return accountProcess{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return accountProcess{}, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var out struct {
		Process accountProcess `json:"process"`
	}
	err = json.NewDecoder(resp.Body).Decode(&out)
	return out.Process, err
}

func followAccountProcess(server, token, processID string) error {
	printed := 0
	for {
		process, err := fetchAccountProcess(server, token, processID)
		if err != nil {
			return err
		}
		if len(process.Output) > printed {
			fmt.Print(process.Output[printed:])
			printed = len(process.Output)
		}
		if process.Status == "exited" {
			if process.ExitCode != nil && *process.ExitCode != 0 {
				return fmt.Errorf("process exited %d", *process.ExitCode)
			}
			return nil
		}
		if process.Status == "lost" {
			return errors.New("process lost")
		}
		time.Sleep(350 * time.Millisecond)
	}
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
		return errors.New("usage: ohrats-rc run [--url URL] [--token TOKEN] DEVICE -- COMMAND [ARG...]")
	}
	flags, server, token, err := accountFlags("ohrats-rc run", args[:separator])
	if err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: ohrats-rc run [flags] DEVICE -- COMMAND")
	}
	device, err := resolveAccountDevice(*server, *token, flags.Arg(0))
	if err != nil {
		return err
	}
	processID, err := startAccountProcess(*server, *token, device.ID, strings.Join(args[separator+1:], " "), "")
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Started %s on %s\n", processID, device.Name)
	return followAccountProcess(*server, *token, processID)
}

func listAccountActions(server, token string) ([]accountAction, error) {
	resp, err := accountRequest(server, token, http.MethodGet, "/api/v1/actions")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var out struct {
		Actions []accountAction `json:"actions"`
	}
	err = json.NewDecoder(resp.Body).Decode(&out)
	return out.Actions, err
}

func actionsCommand(args []string) error {
	flags, server, token, err := accountFlags("ohrats-rc actions", args)
	if err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: ohrats-rc actions [--token TOKEN]")
	}
	actions, err := listAccountActions(*server, *token)
	if err != nil {
		return err
	}
	if len(actions) == 0 {
		fmt.Println("No actions")
		return nil
	}
	for _, action := range actions {
		fmt.Printf("%s  %s  %s\n", action.ID, action.Name, action.Workspace)
	}
	return nil
}

func actionCommand(args []string) error {
	if len(args) == 0 || args[0] != "run" {
		return errors.New("usage: ohrats-rc action run ACTION --device DEVICE [--token TOKEN]")
	}
	var actionValue, deviceValue, server, token string
	server = defaultServer
	token = os.Getenv("RC_API_TOKEN")
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--device":
			i++
			if i < len(args) {
				deviceValue = args[i]
			}
		case "--url":
			i++
			if i < len(args) {
				server = args[i]
			}
		case "--token":
			i++
			if i < len(args) {
				token = args[i]
			}
		default:
			if actionValue == "" {
				actionValue = args[i]
			} else {
				return fmt.Errorf("unexpected argument %q", args[i])
			}
		}
	}
	if actionValue == "" || deviceValue == "" {
		return errors.New("usage: ohrats-rc action run ACTION --device DEVICE")
	}
	actions, err := listAccountActions(server, token)
	if err != nil {
		return err
	}
	var action accountAction
	found := false
	for _, value := range actions {
		if value.ID == actionValue || strings.EqualFold(value.Name, actionValue) || strings.HasPrefix(value.ID, actionValue) {
			if found {
				return fmt.Errorf("action %q is ambiguous", actionValue)
			}
			action = value
			found = true
		}
	}
	if !found {
		return fmt.Errorf("action %q not found", actionValue)
	}
	device, err := resolveAccountDevice(server, token, deviceValue)
	if err != nil {
		return err
	}
	resp, err := accountJSONRequest(server, token, http.MethodPost, "/api/v1/actions/"+url.PathEscape(action.ID)+"/run", map[string]any{"deviceIds": []string{device.ID}})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var out struct {
		Results []struct {
			ProcessID string `json:"processId"`
			Error     string `json:"error"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	if len(out.Results) != 1 {
		return errors.New("unexpected action result")
	}
	if out.Results[0].Error != "" {
		return errors.New(out.Results[0].Error)
	}
	return followAccountProcess(server, token, out.Results[0].ProcessID)
}
