package server

import (
	"strings"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

// UpdateHello refreshes Node-reported version and capabilities.
func (r *DeviceRegistry) UpdateHello(deviceID, version string, capabilities []string) error {
	r.mu.Lock()
	device, ok := r.devices[deviceID]
	if !ok {
		r.mu.Unlock()
		return authstore.ErrDeviceNotFound
	}
	device.Version = strings.TrimSpace(version)
	device.Capabilities = append([]string(nil), capabilities...)
	store := r.store
	r.mu.Unlock()
	if store != nil {
		return store.UpdateDeviceHello(deviceID, version, capabilities)
	}
	return nil
}

// UpdateLockState records the RC Lock generation acknowledged by a Node.
func (r *DeviceRegistry) UpdateLockState(deviceID, hash string, generation uint64) error {
	r.mu.Lock()
	device, ok := r.devices[deviceID]
	if !ok {
		r.mu.Unlock()
		return authstore.ErrDeviceNotFound
	}
	device.LockHash = hash
	device.LockGeneration = generation
	store := r.store
	r.mu.Unlock()
	if store != nil {
		return store.UpdateDeviceLockState(deviceID, hash, generation)
	}
	return nil
}
