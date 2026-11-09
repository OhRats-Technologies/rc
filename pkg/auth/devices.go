package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// ErrDeviceNotFound is returned when a device row does not exist.
var ErrDeviceNotFound = errors.New("device not found")

// Workspace is a durable grouping of devices and members.
type Workspace struct {
	ID        string
	Name      string
	CreatedBy string
	CreatedAt time.Time
}

// Membership grants a user a role in a workspace.
type Membership struct {
	WorkspaceID string
	UserID      string
	Role        string
	JoinedAt    time.Time
}

// Device is an enrolled Node. Keys are base64url without padding.
type Device struct {
	ID                 string
	WorkspaceID        string
	Name               string
	Hostname           string
	Platform           string
	Arch               string
	Version            string
	Capabilities       []string
	IdentityPublicKey  string
	TransportPublicKey string
	LockHash           string
	LockGeneration     int64
	LastSeenAt         *time.Time
	CreatedAt          time.Time
}

// RevokedDevice is the tombstone that lets a Node learn it was removed.
type RevokedDevice struct {
	ID                string
	IdentityPublicKey string
	RevokedAt         time.Time
}

// EnsureWorkspace creates a workspace when it does not already exist.
func (s *Store) EnsureWorkspace(id, name string) error {
	_, err := s.db.ExecContext(context.Background(),
		`INSERT OR IGNORE INTO workspaces (id, name, created_by, created_ms) VALUES (?, ?, NULL, ?);`,
		id, name, nowMs())
	return err
}

// InsertDevice stores a newly enrolled device.
func (s *Store) InsertDevice(device Device) error {
	_, err := s.db.ExecContext(context.Background(), deviceInsertSQL, deviceInsertArgs(device)...)
	return err
}

const deviceInsertSQL = `INSERT INTO devices (id, workspace_id, name, hostname, platform, arch, version, capabilities,
	identity_public_key, transport_public_key, lock_hash, lock_generation, last_seen_ms, created_ms)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

func deviceInsertArgs(device Device) []any {
	capabilities, _ := json.Marshal(nonNilStrings(device.Capabilities))
	return []any{device.ID, device.WorkspaceID, device.Name, device.Hostname, device.Platform, device.Arch,
		device.Version, string(capabilities), device.IdentityPublicKey, device.TransportPublicKey,
		device.LockHash, device.LockGeneration, nullableMs(device.LastSeenAt), msFromTime(device.CreatedAt)}
}

// ListDevices returns every durable device ordered by workspace and name.
func (s *Store) ListDevices() ([]Device, error) {
	rows, err := s.db.QueryContext(context.Background(), deviceSelect+
		` ORDER BY workspace_id, lower(name);`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var devices []Device
	for rows.Next() {
		device, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, rows.Err()
}

// RenameDevice changes a device display name.
func (s *Store) RenameDevice(id, name string) error {
	result, err := s.db.ExecContext(context.Background(),
		`UPDATE devices SET name = ? WHERE id = ?;`, strings.TrimSpace(name), id)
	if err != nil {
		return err
	}
	return requireRow(result)
}

// RevokeDevice writes the revocation tombstone and removes the device in one
// transaction, so an offline Node can learn of its removal on reconnect.
func (s *Store) RevokeDevice(id string) error {
	return s.db.WithTx(context.Background(), func(tx *sql.Tx) error {
		var identity string
		err := tx.QueryRowContext(context.Background(),
			`SELECT identity_public_key FROM devices WHERE id = ?;`, id).Scan(&identity)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrDeviceNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(context.Background(),
			`DELETE FROM authority_deliveries WHERE device_id = ?;`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(context.Background(),
			`INSERT INTO revoked_devices (id, identity_public_key, revoked_ms) VALUES (?, ?, ?);`,
			id, identity, nowMs()); err != nil {
			return err
		}
		_, err = tx.ExecContext(context.Background(), `DELETE FROM devices WHERE id = ?;`, id)
		return err
	})
}

// DeviceRevoked reports whether a device ID has a revocation tombstone.
func (s *Store) DeviceRevoked(id string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM revoked_devices WHERE id = ?;`, id).Scan(&count)
	return count > 0, err
}

// RevokedDevices lists every tombstone, used by tests and diagnostics.
func (s *Store) RevokedDevices() ([]RevokedDevice, error) {
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT id, identity_public_key, revoked_ms FROM revoked_devices ORDER BY revoked_ms, id;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []RevokedDevice
	for rows.Next() {
		var item RevokedDevice
		var revokedMs int64
		if err := rows.Scan(&item.ID, &item.IdentityPublicKey, &revokedMs); err != nil {
			return nil, err
		}
		item.RevokedAt = timeFromMs(revokedMs)
		result = append(result, item)
	}
	return result, rows.Err()
}

const deviceSelect = `SELECT id, workspace_id, name, hostname, platform, arch, version, capabilities,
	identity_public_key, transport_public_key, lock_hash, lock_generation, last_seen_ms, created_ms
	FROM devices`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanDevice(row rowScanner) (Device, error) {
	var device Device
	var capabilities string
	var lastSeen sql.NullInt64
	var createdMs int64
	err := row.Scan(&device.ID, &device.WorkspaceID, &device.Name, &device.Hostname, &device.Platform,
		&device.Arch, &device.Version, &capabilities, &device.IdentityPublicKey, &device.TransportPublicKey,
		&device.LockHash, &device.LockGeneration, &lastSeen, &createdMs)
	if err != nil {
		return Device{}, err
	}
	if err := json.Unmarshal([]byte(capabilities), &device.Capabilities); err != nil {
		return Device{}, err
	}
	if lastSeen.Valid {
		value := timeFromMs(lastSeen.Int64)
		device.LastSeenAt = &value
	}
	device.CreatedAt = timeFromMs(createdMs)
	return device, nil
}

func nullableMs(value *time.Time) any {
	if value == nil {
		return nil
	}
	return msFromTime(*value)
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func requireRow(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

// UpdateDeviceHello refreshes Node-reported version and capabilities.
func (s *Store) UpdateDeviceHello(id, version string, capabilities []string) error {
	encoded, _ := json.Marshal(nonNilStrings(capabilities))
	result, err := s.db.ExecContext(context.Background(),
		`UPDATE devices SET version = ?, capabilities = ? WHERE id = ?;`,
		strings.TrimSpace(version), string(encoded), id)
	if err != nil {
		return err
	}
	return requireRow(result)
}
