package server

import (
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
)

// defaultWorkspaceID is the workspace used by development enrollment tokens.
const defaultWorkspaceID = "ws_default"

// attachStore makes the registry durable. The in-memory map becomes a cache of
// the store contents and every mutation is written to the store first.
func (r *DeviceRegistry) attachStore(store *authstore.Store) error {
	devices, err := store.ListDevices()
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store = store
	r.devices = make(map[string]*RegisteredDevice, len(devices))
	for _, device := range devices {
		r.devices[device.ID] = registeredFromStore(device)
	}
	return nil
}

func registeredFromStore(device authstore.Device) *RegisteredDevice {
	registered := &RegisteredDevice{
		ID:                 device.ID,
		WorkspaceID:        device.WorkspaceID,
		Name:               device.Name,
		Hostname:           device.Hostname,
		Platform:           device.Platform,
		Arch:               device.Arch,
		Version:            device.Version,
		Capabilities:       device.Capabilities,
		IdentityPublicKey:  device.IdentityPublicKey,
		TransportPublicKey: device.TransportPublicKey,
		LockHash:           device.LockHash,
		LockGeneration:     uint64(device.LockGeneration),
	}
	if device.LastSeenAt != nil {
		registered.LastSeen = *device.LastSeenAt
	}
	return registered
}

// enrolledDevice converts a verified enrollment into a durable device record.
func enrolledDevice(device *RegisteredDevice, createdAt time.Time) authstore.Device {
	return authstore.Device{
		ID:                 device.ID,
		WorkspaceID:        device.WorkspaceID,
		Name:               device.Name,
		Hostname:           device.Hostname,
		Platform:           device.Platform,
		Arch:               device.Arch,
		Version:            device.Version,
		Capabilities:       device.Capabilities,
		IdentityPublicKey:  device.IdentityPublicKey,
		TransportPublicKey: device.TransportPublicKey,
		CreatedAt:          createdAt,
	}
}

// persistEnrollment writes a new device before it becomes visible. The
// development workspace is created on first use so the device foreign key holds.
func (r *DeviceRegistry) persistEnrollment(device *RegisteredDevice) error {
	if r.store == nil {
		return nil
	}
	if device.WorkspaceID == defaultWorkspaceID {
		if err := r.store.EnsureWorkspace(defaultWorkspaceID, "Default"); err != nil {
			return err
		}
	}
	return r.store.InsertDevice(enrolledDevice(device, time.Now().UTC()))
}

// remove revokes a device durably, then drops it from the cache.
func (r *DeviceRegistry) remove(id string) error {
	if r.store != nil {
		if err := r.store.RevokeDevice(id); err != nil {
			return err
		}
	}
	r.mu.Lock()
	delete(r.devices, id)
	r.mu.Unlock()
	return nil
}
