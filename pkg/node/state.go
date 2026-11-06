package node

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OhRats-Technologies/rc/pkg/platform"
)

const StateVersion uint8 = 1

var b64URL = base64.RawURLEncoding

type NodeState struct {
	V               uint8  `json:"v"`
	DeviceID        string `json:"deviceId"`
	IdentitySeed    string `json:"identitySeed"`
	TransportSecret string `json:"transportSecret"`
}

type NodeConfig struct {
	Server string `json:"server,omitempty"`
	Name   string `json:"name,omitempty"`
}

type AccountSession struct {
	Server      string `json:"server"`
	ClientID    string `json:"clientId"`
	SigningSeed string `json:"signingSeed"`
	User        string `json:"user,omitempty"`
}

func GenerateNodeState(deviceID string) (*NodeState, error) {
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, err
	}
	transport := make([]byte, 32)
	if _, err := rand.Read(transport); err != nil {
		return nil, err
	}
	return &NodeState{
		V:               StateVersion,
		DeviceID:        deviceID,
		IdentitySeed:    b64URL.EncodeToString(priv.Seed()),
		TransportSecret: b64URL.EncodeToString(transport),
	}, nil
}

func (s *NodeState) IdentityPublicKey() (string, error) {
	seed, err := b64URL.DecodeString(s.IdentitySeed)
	if err != nil || len(seed) != 32 {
		return "", errors.New("invalid identity seed")
	}
	key := ed25519.NewKeyFromSeed(seed)
	pub := key.Public().(ed25519.PublicKey)
	return b64URL.EncodeToString(pub), nil
}

func (s *NodeState) TransportPublicKey() (string, error) {
	sec, err := b64URL.DecodeString(s.TransportSecret)
	if err != nil || len(sec) != 32 {
		return "", errors.New("invalid transport secret")
	}
	k, err := ecdh.X25519().NewPrivateKey(sec)
	if err != nil {
		return "", err
	}
	return b64URL.EncodeToString(k.PublicKey().Bytes()), nil
}

func (s *NodeState) Validate() error {
	if s.V != StateVersion || s.DeviceID == "" {
		return errors.New("invalid RC Node state")
	}
	if _, err := s.IdentityPublicKey(); err != nil {
		return err
	}
	if _, err := s.TransportPublicKey(); err != nil {
		return err
	}
	return nil
}

func ResolveStateDir(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return filepath.Clean(explicit), nil
	}
	return platform.StateDir()
}

func StatePath(dir string) string   { return filepath.Join(dir, "device.json") }
func ConfigPath(dir string) string  { return filepath.Join(dir, "config.json") }
func AccountPath(dir string) string { return filepath.Join(dir, "account.json") }
func LockPath(dir string) string    { return filepath.Join(dir, "lock.json") }

func LoadState(dir string) (*NodeState, error) {
	var s NodeState
	if err := loadJSON(StatePath(dir), &s); err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

func SaveState(dir string, s *NodeState) error {
	if err := s.Validate(); err != nil {
		return err
	}
	return saveJSON(dir, StatePath(dir), s)
}

func LoadConfig(dir string) (*NodeConfig, error) {
	var c NodeConfig
	err := loadJSON(ConfigPath(dir), &c)
	if errors.Is(err, os.ErrNotExist) {
		return &NodeConfig{}, nil
	}
	return &c, err
}

func SaveConfig(dir string, c *NodeConfig) error {
	return saveJSON(dir, ConfigPath(dir), c)
}

func LoadAccount(dir string) (*AccountSession, error) {
	var a AccountSession
	err := loadJSON(AccountPath(dir), &a)
	return &a, err
}

func SaveAccount(dir string, a *AccountSession) error {
	return saveJSON(dir, AccountPath(dir), a)
}

func loadJSON(path string, out any) error {
	if err := validatePrivateFile(path); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func saveJSON(dir, path string, val any) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	_ = platform.ProtectPrivatePath(dir, true)

	data, err := json.MarshalIndent(val, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp := fmt.Sprintf("%s.tmp-%d", path, os.Getpid())
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	_ = platform.ProtectPrivatePath(tmp, false)
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return platform.ProtectPrivatePath(path, false)
}
