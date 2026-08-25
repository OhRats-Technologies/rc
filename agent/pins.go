package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type devicePin struct {
	IdentityKey  string `json:"identityKey"`
	TransportKey string `json:"transportKey"`
}

func pinsPath(dir string) string { return filepath.Join(dir, "device-pins.json") }

func loadDevicePins(dir string) (map[string]devicePin, error) {
	pins := map[string]devicePin{}
	data, err := os.ReadFile(pinsPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return pins, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &pins); err != nil {
		return nil, err
	}
	return pins, nil
}

func saveDevicePins(dir string, pins map[string]devicePin) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(pins, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(pinsPath(dir), data, 0600)
}

func verifyDevicePin(dir string, device accountDevice) error {
	if device.ID == "" || device.IdentityPublicKey == "" || device.TransportPublicKey == "" {
		return errors.New("RC Node cryptographic identity is unavailable")
	}
	pins, err := loadDevicePins(dir)
	if err != nil {
		return err
	}
	next := devicePin{IdentityKey: device.IdentityPublicKey, TransportKey: device.TransportPublicKey}
	if current, ok := pins[device.ID]; ok {
		if current != next {
			return errors.New("RC Node cryptographic identity changed; re-enroll the device before trusting it again")
		}
		return nil
	}
	pins[device.ID] = next
	return saveDevicePins(dir, pins)
}
